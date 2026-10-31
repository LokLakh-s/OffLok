//go:build cgo && (android || libunbound)

// Package mobile is OffLok's core inside the Android app, built with gomobile bind: the app (Kotlin) starts it,
// hands it the network the phone uses, and asks it what the window asks; the core runs Unbound (libunbound),
// OffLok's front, and the VPN tunnel that carries the phone's DNS queries to them.
package mobile

import (
	"context"
	"encoding/json"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loklakh-s/offlok/internal/blocklist"
	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/libunbound"
	"github.com/loklakh-s/offlok/internal/settings"
)

// The core's ports, on the phone's loopback: the front's (Android lets no app take port 53) and Unbound's.
const (
	frontPort   = 10053
	unboundPort = 10530
)

var core struct {
	sync.Mutex
	d      *daemon.Daemon
	b      *backend
	cancel context.CancelFunc
	done   chan struct{}
}

type logWriter struct{ p Platform }

func (w logWriter) Write(b []byte) (int, error) {
	w.p.Log(strings.TrimRight(string(b), "\n"))
	return len(b), nil
}

// Start runs the core, keeping its files in dir (the app's own). version is the app's. Started already, it does
// nothing.
func Start(dir, version string, p Platform) error {
	core.Lock()
	defer core.Unlock()
	if core.d != nil {
		return nil
	}
	logger := log.New(logWriter{p}, "", 0)
	path := filepath.Join(dir, "settings.json")
	if err := firstSettings(path); err != nil {
		return err
	}
	// libunbound starts only with a bundle it can read: without one, OffLok runs without DNS-over-TLS.
	bundle := filepath.Join(dir, "ca-bundle.pem")
	if err := writeCABundle(bundle); err != nil {
		logger.Printf("the certificate authorities for DNS-over-TLS: %v", err)
		if _, err := os.Stat(bundle); err != nil {
			bundle = ""
		}
	}
	b := &backend{platform: p, log: logger}
	userAgent := "OffLok/" + version + " (Android; +https://github.com/LokLakh-s/OffLok)"
	d := &daemon.Daemon{
		Paths:          daemon.Paths{Settings: path, State: dir, Unbound: filepath.Join(dir, "unbound")},
		Backend:        b,
		Engine:         libunbound.New,
		UnboundVersion: libunbound.Version(),
		CABundle:       bundle,
		Port:           frontPort,
		UnboundPort:    unboundPort,
		UnboundSpare:   []uint16{20530, 30530, 40530},
		Lists: &blocklist.Store{Dir: filepath.Join(dir, "lists"), Log: logger, UserAgent: userAgent,
			Client: listsClient(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), frontPort))},
		Version:     version,
		RootServers: daemon.DefaultRootServers(),
		Log:         logger,
		Tick:        30 * time.Second,
		Changed: func(st daemon.Status) {
			if data, err := json.Marshal(st); err == nil {
				p.Changed(string(data))
			}
		},
	}
	b.answer = d.Answer
	b.lost = func() {
		_ = d.Update(func(s *settings.Settings) error { s.Enabled = false; return nil })
	}
	if err := d.Init(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()
	core.d, core.b, core.cancel, core.done = d, b, cancel, done
	logger.Printf("OffLok %s, Unbound %s", version, libunbound.Version())
	return nil
}

// firstSettings writes the settings of a first start: off, until the user turns OffLok on and lets it start a VPN.
// Updates come from the app's store.
func firstSettings(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	s := settings.Default()
	s.Enabled, s.Updates = false, false
	return settings.Save(path, s)
}

// Stop ends the core: the tunnel closes, Unbound stops.
func Stop() {
	core.Lock()
	defer core.Unlock()
	if core.d == nil {
		return
	}
	core.cancel()
	<-core.done
	core.d, core.b = nil, nil
}

// result is what the window gets, as the computers' app gives it (desktop/main.go's Result).
type result struct {
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	NotRunning bool            `json:"not_running,omitempty"`
	Status     json.RawMessage `json:"status,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// Call runs one of the core's commands (status, on, off, strict, dot, block, trust, check...), as the window
// sends them, and returns the result as JSON. The phone's user is its administrator.
func Call(cmd, value string) string {
	core.Lock()
	d, b := core.d, core.b
	core.Unlock()
	var r result
	if d == nil {
		r = result{NotRunning: true, Error: "OffLok's core is not running"}
	} else {
		if cmd == "on" {
			b.turningOn()
		}
		resp := d.Handle(ipc.Request{Cmd: cmd, Value: value}, ipc.Peer{Admin: true})
		r = result{OK: resp.OK, Error: resp.Error, Status: resp.Status, Data: resp.Data}
	}
	data, _ := json.Marshal(r)
	return string(data)
}

// SetNetwork tells the core the network the phone uses: its identity (any text that changes with it), its DNS
// servers and search domains, one per line, and its Private DNS hostname when one is set.
func SetNetwork(id, servers, domains, privateDNS string) {
	core.Lock()
	d, b := core.d, core.b
	core.Unlock()
	if d == nil {
		return
	}
	b.setNetwork(id, networkOf(servers, domains), privateDNS)
	d.Wake()
}

// Revoked says Android took the VPN from OffLok (VpnService.onRevoke): another VPN app started, or the user
// turned OffLok's VPN off in Android's settings. OffLok turns off, and the window says why.
func Revoked() {
	core.Lock()
	b := core.b
	core.Unlock()
	if b != nil {
		b.revoked()
	}
}

// UnboundVersion is the version of the Unbound inside the app.
func UnboundVersion() string { return libunbound.Version() }
