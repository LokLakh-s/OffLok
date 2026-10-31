package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/loklakh-s/offlok/internal/blocklist"
	"github.com/loklakh-s/offlok/internal/settings"
	"github.com/loklakh-s/offlok/internal/sysdns"
)

const (
	pauseFor      = 10 * time.Minute
	listsInterval = time.Hour // how often the lists loop looks whether a list is a day old
)

// BlockingStatus is the blocking's part of the status.
type BlockingStatus struct {
	Categories  []blocklist.Category   `json:"categories"` // those this build offers: no ads on Google Play
	PausedUntil *time.Time             `json:"paused_until,omitempty"`
	Lists       []blocklist.SourceInfo `json:"lists,omitempty"`
}

// Check is what "Is this site blocked?" answers.
type Check struct {
	Name    string           `json:"name"`
	Blocked bool             `json:"blocked"`
	Trusted bool             `json:"trusted,omitempty"`
	Paused  bool             `json:"paused,omitempty"`
	Match   *blocklist.Match `json:"match,omitempty"`
}

func selection(s settings.Settings) []blocklist.Selected {
	enabled := map[blocklist.Category]bool{}
	for c, on := range s.Blocking {
		enabled[blocklist.Category(c)] = on
	}
	return blocklist.Selection(enabled, s.CustomLists)
}

// blocked is the front's question, for every query: not while paused, never a trusted site, otherwise the lists.
func (d *Daemon) blocked(name string) bool {
	d.mu.Lock()
	paused, trusted := time.Now().Before(d.pause), d.trusted
	d.mu.Unlock()
	if paused || trusted.Covers(name) != "" || d.Lists == nil {
		return false
	}
	_, ok := d.Lists.Lookup(name)
	return ok
}

// listsLoop keeps the lists the settings select loaded: at start (from disk, so blocking works without a network),
// on every change of the settings, and once an hour, downloading those a day old.
func (d *Daemon) listsLoop(ctx context.Context) {
	t := time.NewTicker(listsInterval)
	defer t.Stop()
	var loaded []blocklist.Selected
	for {
		sel := selection(d.currentSettingsNoMark())
		if d.Lists.Due(sel) {
			rctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := d.Lists.Refresh(rctx, sel); err != nil {
				d.Log.Printf("blocklists: %v", err)
			}
			cancel()
			loaded = nil // reload what was downloaded
		}
		if !slices.Equal(sel, loaded) {
			if err := d.Lists.Load(sel); err != nil {
				d.Log.Printf("blocklists: load: %v", err)
			} else {
				loaded = sel
				d.flushCaches()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-d.listWake:
		}
	}
}

// currentSettingsNoMark reads the settings without taking the main loop's "changed" mark.
func (d *Daemon) currentSettingsNoMark() settings.Settings {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.settings.Clone()
}

// flushCaches empties the system resolver's cache, so that a change of the blocking shows at once.
func (d *Daemon) flushCaches() {
	if f, ok := d.Backend.(sysdns.Flusher); ok {
		if err := f.FlushCaches(); err != nil {
			d.Log.Printf("flush the system resolver's cache: %v", err)
		}
	}
}

// Pause stops the blocking for 10 minutes; "off" ends a pause.
func (d *Daemon) Pause(value string) error {
	d.mu.Lock()
	if value == "off" {
		d.pause = time.Time{}
	} else {
		d.pause = time.Now().Add(pauseFor)
	}
	d.mu.Unlock()
	d.flushCaches()
	return nil
}

// siteName reads what someone typed or pasted (a name, or an address such as https://www.example.org/page) as a
// domain name.
func siteName(input string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", err
		}
		s = u.Hostname()
	} else if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s, _, _ = strings.Cut(s, "/")
	s = strings.TrimSuffix(s, ".")
	if !blocklist.ValidName(s) {
		return "", fmt.Errorf("%q is not a site name", input)
	}
	return s, nil
}

// CheckSite says whether name is blocked, by which list, and whether it is trusted.
func (d *Daemon) CheckSite(input string) (Check, error) {
	name, err := siteName(input)
	if err != nil {
		return Check{}, err
	}
	d.mu.Lock()
	c := Check{Name: name, Paused: time.Now().Before(d.pause), Trusted: d.trusted.Covers(name) != ""}
	d.mu.Unlock()
	if d.Lists != nil {
		if m, ok := d.Lists.Lookup(name); ok {
			c.Match = &m
			c.Blocked = !c.Trusted && !c.Paused
		}
	}
	return c, nil
}

// Trust adds a site to those never blocked, or takes it off (trust false).
func (d *Daemon) Trust(input string, trust bool) error {
	name, err := siteName(input)
	if err != nil {
		return err
	}
	err = d.Update(func(s *settings.Settings) error {
		i := slices.Index(s.Trusted, name)
		switch {
		case trust && i < 0:
			s.Trusted = append(s.Trusted, name)
			slices.Sort(s.Trusted)
		case !trust && i >= 0:
			s.Trusted = slices.Delete(s.Trusted, i, i+1)
		}
		return nil
	})
	d.flushCaches()
	return err
}

// SetBlock switches a category on or off: "ads=on".
func (d *Daemon) SetBlock(value string) error {
	cat, state, ok := strings.Cut(value, "=")
	if !ok || !slices.Contains(blocklist.Categories, blocklist.Category(cat)) {
		return fmt.Errorf("expected <category>=on|off, with a category among %v", blocklist.Categories)
	}
	on, err := parseOnOff(state)
	if err != nil {
		return err
	}
	return d.Update(func(s *settings.Settings) error {
		if s.Blocking == nil {
			s.Blocking = map[string]bool{}
		}
		if on {
			s.Blocking[cat] = true
		} else {
			delete(s.Blocking, cat)
		}
		return nil
	})
}

// CustomList adds or removes a list by its address: "add <url>", "remove <url>".
func (d *Daemon) CustomList(value string) error {
	verb, raw, _ := strings.Cut(strings.TrimSpace(value), " ")
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("%q is not the address of a list (https://…)", raw)
	}
	switch verb {
	case "add":
		return d.Update(func(s *settings.Settings) error {
			if !slices.Contains(s.CustomLists, raw) {
				s.CustomLists = append(s.CustomLists, raw)
			}
			return nil
		})
	case "remove":
		return d.Update(func(s *settings.Settings) error {
			i := slices.Index(s.CustomLists, raw)
			if i < 0 {
				return errors.New("no such list")
			}
			s.CustomLists = slices.Delete(s.CustomLists, i, i+1)
			return nil
		})
	}
	return errors.New("expected add <url> or remove <url>")
}
