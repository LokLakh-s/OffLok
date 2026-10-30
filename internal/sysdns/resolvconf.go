//go:build linux || darwin

package sysdns

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const resolvConfMarker = "# Written by OffLok"

// ResolvConf writes /etc/resolv.conf itself, for systems where no resolver daemon owns it. The original file, or
// the symbolic link it was, is kept in a state file and put back by Restore. When something else rewrites the file
// (a DHCP client, NetworkManager), that new content becomes the original: it is the network's latest settings.
type ResolvConf struct {
	Path      string // /etc/resolv.conf unless testing
	StatePath string // where the original is kept
}

type resolvConfState struct {
	Symlink string `json:"symlink,omitempty"` // the original was a symbolic link to this target
	Content string `json:"content"`           // the original content (the target's, for a link)
}

func (r *ResolvConf) Name() string { return "resolv.conf" }

func (r *ResolvConf) path() string {
	if r.Path != "" {
		return r.Path
	}
	return "/etc/resolv.conf"
}

func renderResolvConf(servers []netip.Addr, domains []string) string {
	var b strings.Builder
	b.WriteString(resolvConfMarker + ": the original comes back when OffLok is turned off or uninstalled.\n")
	for _, a := range servers {
		b.WriteString("nameserver " + a.String() + "\n")
	}
	if len(domains) > 0 {
		b.WriteString("search " + strings.Join(domains, " ") + "\n")
	}
	b.WriteString("options edns0 trust-ad\n")
	return b.String()
}

func (r *ResolvConf) loadState() (*resolvConfState, error) {
	data, err := os.ReadFile(r.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st resolvConfState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", r.StatePath, err)
	}
	return &st, nil
}

// current reads what is at path now: a link target (if any) and the content.
func (r *ResolvConf) current() (resolvConfState, error) {
	var st resolvConfState
	if target, err := os.Readlink(r.path()); err == nil {
		st.Symlink = target
	}
	data, err := os.ReadFile(r.path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return st, err
	}
	st.Content = string(data)
	return st, nil
}

func (r *ResolvConf) Apply(servers []netip.Addr) error {
	cur, err := r.current()
	if err != nil {
		return err
	}
	saved, err := r.loadState()
	if err != nil {
		return err
	}
	if cur.Symlink != "" || !strings.HasPrefix(cur.Content, resolvConfMarker) {
		saved = &cur
		data, _ := json.Marshal(saved)
		if err := writeFileAtomic(r.StatePath, data, 0o600); err != nil {
			return err
		}
	}
	if saved == nil {
		return errors.New("resolv.conf was written by OffLok but its original is lost")
	}
	want := renderResolvConf(servers, ParseResolvConf(saved.Content).Domains)
	if cur.Symlink == "" && cur.Content == want {
		return nil
	}
	return replaceFile(r.path(), []byte(want), 0o644)
}

func (r *ResolvConf) Applied(servers []netip.Addr) (bool, error) {
	cur, err := r.current()
	if err != nil {
		return false, err
	}
	return cur.Symlink == "" && strings.HasPrefix(cur.Content, resolvConfMarker) &&
		slices.Equal(ParseResolvConf(cur.Content).Servers, servers), nil
}

func (r *ResolvConf) Restore() error {
	saved, err := r.loadState()
	if err != nil || saved == nil {
		return err
	}
	cur, err := r.current()
	if err != nil {
		return err
	}
	if cur.Symlink == "" && strings.HasPrefix(cur.Content, resolvConfMarker) {
		if saved.Symlink != "" {
			tmp := r.path() + ".offlok-tmp"
			_ = os.Remove(tmp)
			if err := os.Symlink(saved.Symlink, tmp); err != nil {
				return err
			}
			if err := os.Rename(tmp, r.path()); err != nil {
				_ = os.Remove(tmp)
				return err
			}
		} else if err := replaceFile(r.path(), []byte(saved.Content), 0o644); err != nil {
			return err
		}
	}
	// Otherwise someone replaced our file since: theirs stays.
	return os.Remove(r.StatePath)
}

func (r *ResolvConf) Network() (Network, error) {
	saved, err := r.loadState()
	if err != nil {
		return Network{}, err
	}
	if saved == nil {
		cur, err := r.current()
		if err != nil {
			return Network{}, err
		}
		saved = &cur
	}
	return ParseResolvConf(saved.Content).WithoutLoopback(), nil
}

// replaceFile writes path atomically, or in place when path is a mount point (Docker bind-mounts /etc/resolv.conf,
// and a rename over a mount point fails).
func replaceFile(path string, data []byte, perm os.FileMode) error {
	if err := writeFileAtomic(path, data, perm); err == nil {
		return nil
	}
	return os.WriteFile(path, data, perm)
}

// writeFileAtomic writes data to a temporary file next to path, then renames it over path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
