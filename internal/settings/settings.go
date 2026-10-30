// Package settings holds what the user chose, as offlokd keeps it on disk.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Access says who may change settings. Seeing the status is always open to everyone.
type Access string

const (
	AdminsOnly Access = "admins"
	Everyone   Access = "everyone"
)

// DoT is the personal DNS-over-TLS resolver used when the root servers are out of reach.
type DoT struct {
	Name       string       `json:"name"` // host name its certificate carries
	Port       uint16       `json:"port"`
	Addrs      []netip.Addr `json:"addrs"` // looked up when it was set, refreshed while the root is reachable
	ResolvedAt time.Time    `json:"resolved_at"`
}

// Settings are the user's choices.
type Settings struct {
	Enabled bool   `json:"enabled"`
	Strict  bool   `json:"strict"` // never fall back to the network's DNS
	DoT     *DoT   `json:"dot,omitempty"`
	Access  Access `json:"settings_access"`
	// Blocking: the categories switched on (ads, trackers, malicious, custom), all off at first.
	Blocking    map[string]bool `json:"blocking,omitempty"`
	Trusted     []string        `json:"trusted,omitempty"`      // sites never blocked, with what is under them
	CustomLists []string        `json:"custom_lists,omitempty"` // the addresses of lists the user added
	// Updates: look for a new release once a day on GitHub, which then sees the computer's address. On at first.
	Updates bool `json:"updates"`
}

// Clone copies s, its maps and slices included: the copy can change without touching s.
func (s Settings) Clone() Settings {
	if s.DoT != nil {
		dot := *s.DoT
		dot.Addrs = slices.Clone(dot.Addrs)
		s.DoT = &dot
	}
	s.Blocking = maps.Clone(s.Blocking)
	s.Trusted = slices.Clone(s.Trusted)
	s.CustomLists = slices.Clone(s.CustomLists)
	return s
}

// Default is what a fresh install starts with. A file written before a setting existed keeps that setting's
// default (Load starts from here).
func Default() Settings { return Settings{Enabled: true, Access: AdminsOnly, Updates: true} }

// Load reads the settings at path; a missing file gives the defaults.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	s := Default()
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.Access != Everyone {
		s.Access = AdminsOnly
	}
	return s, nil
}

// Save writes the settings to path atomically.
func Save(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
