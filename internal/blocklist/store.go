package blocklist

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxListSize = 64 << 20 // no list is near this; a bigger answer is not a list
	refreshAge  = 24 * time.Hour
)

// Selected is one source to use, with its category.
type Selected struct {
	Category Category
	Source   Source
}

// Selection is what the settings ask for: the enabled categories' sources, and the custom lists if that category
// is on.
func Selection(enabled map[Category]bool, custom []string) []Selected {
	var out []Selected
	for _, c := range Categories {
		if !enabled[c] {
			continue
		}
		if c == Custom {
			for _, u := range custom {
				out = append(out, Selected{Custom, CustomSource(u)})
			}
			continue
		}
		for _, s := range Sources[c] {
			out = append(out, Selected{c, s})
		}
	}
	return out
}

// Meta is what the store remembers of one source between downloads.
type Meta struct {
	URL       string    `json:"url"`
	ETag      string    `json:"etag,omitempty"`
	Modified  string    `json:"last_modified,omitempty"`
	CheckedAt time.Time `json:"checked_at"` // the last time the source answered
	Names     int       `json:"names"`
	Error     string    `json:"error,omitempty"` // the last download's failure, cleared by a success
}

type loaded struct {
	Selected
	block, allow *Set
}

// Match says why a name is blocked.
type Match struct {
	Category Category `json:"category"`
	Source   string   `json:"source"` // the list's name
	Entry    string   `json:"entry"`  // the listed domain that covers the name
}

// Store keeps the lists on disk (normalized: one name per line, "@@" before an exception) and the enabled ones in
// memory, compiled.
type Store struct {
	Dir       string
	Client    *http.Client
	UserAgent string
	Log       *log.Logger

	mu     sync.RWMutex
	meta   map[string]Meta
	active []loaded
}

func (s *Store) metaPath() string           { return filepath.Join(s.Dir, "lists.json") }
func (s *Store) listPath(src Source) string { return filepath.Join(s.Dir, src.ID+".list") }
func (s *Store) logf(f string, args ...any) { s.Log.Printf("blocklists: "+f, args...) }
func (s *Store) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: time.Minute}
}

func (s *Store) loadMeta() {
	if s.meta != nil {
		return
	}
	s.meta = map[string]Meta{}
	if data, err := os.ReadFile(s.metaPath()); err == nil {
		_ = json.Unmarshal(data, &s.meta)
	}
}

func (s *Store) saveMeta() error {
	data, err := json.MarshalIndent(s.meta, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.metaPath(), data)
}

// Due reports whether a selected source has never been downloaded, or not checked for a day.
func (s *Store) Due(sel []Selected) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMeta()
	for _, x := range sel {
		m, ok := s.meta[x.Source.ID]
		if !ok || time.Since(m.CheckedAt) > refreshAge {
			return true
		}
		if _, err := os.Stat(s.listPath(x.Source)); err != nil {
			return true
		}
	}
	return false
}

// Refresh downloads the selected sources that are due, asking the server whether they changed first. A failed
// download keeps the previous copy.
func (s *Store) Refresh(ctx context.Context, sel []Selected) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	var errs []error
	for _, x := range sel {
		s.mu.Lock()
		s.loadMeta()
		m := s.meta[x.Source.ID]
		s.mu.Unlock()
		_, statErr := os.Stat(s.listPath(x.Source))
		if statErr == nil && m.URL == x.Source.URL && time.Since(m.CheckedAt) < refreshAge {
			continue
		}
		if statErr != nil || m.URL != x.Source.URL {
			m = Meta{URL: x.Source.URL}
		}
		next, err := s.fetch(ctx, x.Source, m)
		if err != nil {
			next.Error = err.Error()
			errs = append(errs, fmt.Errorf("%s: %w", x.Source.Name, err))
		}
		s.mu.Lock()
		s.meta[x.Source.ID] = next
		saveErr := s.saveMeta()
		s.mu.Unlock()
		if saveErr != nil {
			errs = append(errs, saveErr)
		}
	}
	return errors.Join(errs...)
}

func (s *Store) fetch(ctx context.Context, src Source, m Meta) (Meta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return m, err
	}
	if s.UserAgent != "" {
		req.Header.Set("User-Agent", s.UserAgent)
	}
	if m.ETag != "" {
		req.Header.Set("If-None-Match", m.ETag)
	}
	if m.Modified != "" {
		req.Header.Set("If-Modified-Since", m.Modified)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		m.CheckedAt, m.Error = time.Now(), ""
		return m, nil
	case http.StatusOK:
	default:
		return m, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxListSize+1))
	if err != nil {
		return m, err
	}
	if len(body) > maxListSize {
		return m, errors.New("more than 64 MB: not a list")
	}
	p, err := Parse(bytes.NewReader(body))
	if err != nil {
		return m, err
	}
	var b strings.Builder
	for _, n := range p.Block {
		b.WriteString(n + "\n")
	}
	for _, n := range p.Allow {
		b.WriteString("@@" + n + "\n")
	}
	if err := writeAtomic(s.listPath(src), []byte(b.String())); err != nil {
		return m, err
	}
	s.logf("%s: %d names (%d rules skipped)", src.Name, len(p.Block), p.Skipped)
	return Meta{URL: src.URL, ETag: resp.Header.Get("ETag"), Modified: resp.Header.Get("Last-Modified"),
		CheckedAt: time.Now(), Names: len(p.Block)}, nil
}

// Load compiles the selected sources from disk, replacing what was in memory. Sources never downloaded are left out.
func (s *Store) Load(sel []Selected) error {
	var out []loaded
	for _, x := range sel {
		f, err := os.Open(s.listPath(x.Source))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var block, allow []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if name, ok := strings.CutPrefix(sc.Text(), "@@"); ok {
				allow = append(allow, name)
			} else if t := sc.Text(); t != "" {
				block = append(block, t)
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return err
		}
		out = append(out, loaded{Selected: x, block: NewSet(block), allow: NewSet(allow)})
	}
	s.mu.Lock()
	s.active = out
	s.mu.Unlock()
	return nil
}

// Lookup reports whether a list blocks name, and which one. A list's exception (@@) allows the name again, whatever
// list blocked it, as AdGuard does.
func (s *Store) Lookup(name string) (Match, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.active {
		if l.allow.Covers(name) != "" {
			return Match{}, false
		}
	}
	for _, l := range s.active {
		if e := l.block.Covers(name); e != "" {
			return Match{Category: l.Category, Source: l.Source.Name, Entry: e}, true
		}
	}
	return Match{}, false
}

// SourceInfo is one active source, as the status shows it.
type SourceInfo struct {
	Category  Category  `json:"category"`
	Name      string    `json:"name"`
	Names     int       `json:"names"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
	Error     string    `json:"error,omitempty"`
}

// Info lists the selected sources with what the store knows of them.
func (s *Store) Info(sel []Selected) []SourceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMeta()
	var out []SourceInfo
	for _, x := range sel {
		m := s.meta[x.Source.ID]
		out = append(out, SourceInfo{Category: x.Category, Name: x.Source.Name, Names: m.Names, CheckedAt: m.CheckedAt, Error: m.Error})
	}
	return out
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
