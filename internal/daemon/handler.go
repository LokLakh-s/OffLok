package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/settings"
)

// freeCommands need no rights: the status, getting online behind a captive portal, pausing the blocking and
// asking whether a site is blocked. Everything else changes settings: administrators only, unless an administrator
// opened settings to everyone.
var freeCommands = map[string]bool{"status": true, "signin": true, "pause": true, "check": true}

// Handle answers one request from the offlok command or the tray app.
func (d *Daemon) Handle(req ipc.Request, peer ipc.Peer) ipc.Response {
	if !freeCommands[req.Cmd] && !d.allowed(req, peer) {
		return d.respond(errDenied)
	}
	switch req.Cmd {
	case "status":
		return d.respond(nil)
	case "signin":
		return d.respond(d.Signin())
	case "pause":
		return d.respond(d.Pause(req.Value))
	case "check":
		c, err := d.CheckSite(req.Value)
		r := d.respond(err)
		if err == nil {
			r.Data, _ = json.Marshal(c)
		}
		return r
	case "block":
		return d.respond(d.SetBlock(req.Value))
	case "trust", "untrust":
		return d.respond(d.Trust(req.Value, req.Cmd == "trust"))
	case "list":
		return d.respond(d.CustomList(req.Value))
	case "on", "off":
		return d.respond(d.Update(func(s *settings.Settings) error { s.Enabled = req.Cmd == "on"; return nil }))
	case "strict":
		on, err := parseOnOff(req.Value)
		if err != nil {
			return d.respond(err)
		}
		return d.respond(d.Update(func(s *settings.Settings) error { s.Strict = on; return nil }))
	case "updates":
		on, err := parseOnOff(req.Value)
		if err != nil {
			return d.respond(err)
		}
		return d.respond(d.Update(func(s *settings.Settings) error { s.Updates = on; return nil }))
	case "update":
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return d.respond(d.InstallUpdate(ctx))
	case "dot":
		if req.Value == "" || req.Value == "none" {
			return d.respond(d.SetDoT(context.Background(), "", 0))
		}
		name, port, err := parseHostPort(req.Value)
		if err != nil {
			return d.respond(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return d.respond(d.SetDoT(ctx, name, port))
	case "access":
		a := settings.Access(req.Value)
		if a != settings.AdminsOnly && a != settings.Everyone {
			return d.respond(fmt.Errorf("access must be %q or %q", settings.AdminsOnly, settings.Everyone))
		}
		// Opening or closing settings stays an administrator's decision, even once they are open.
		if !peer.Admin && !d.authorized(req, peer) {
			return d.respond(errDenied)
		}
		return d.respond(d.Update(func(s *settings.Settings) error { s.Access = a; return nil }))
	default:
		return d.respond(fmt.Errorf("unknown command %q", req.Cmd))
	}
}

var errDenied = errors.New("administrator rights needed")

// allowed reports whether peer may change settings: root, anyone once settings are open to everyone, or a user
// polkit lets through (the administrator's password, asked in their session).
func (d *Daemon) allowed(req ipc.Request, peer ipc.Peer) bool {
	return peer.Admin || d.currentAccess() == settings.Everyone || d.authorized(req, peer)
}

func (d *Daemon) authorized(req ipc.Request, peer ipc.Peer) bool {
	if !req.Interactive || d.Authorize == nil {
		return false
	}
	ok, err := d.Authorize(peer)
	if err != nil {
		d.Log.Printf("authorization: %v", err)
	}
	return ok
}

func (d *Daemon) currentAccess() settings.Access {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.settings.Access
}

func (d *Daemon) respond(err error) ipc.Response {
	st, _ := json.Marshal(d.Status())
	r := ipc.Response{OK: err == nil, Status: st}
	if err != nil {
		r.Error, r.Denied = err.Error(), errors.Is(err, errDenied)
	}
	return r
}

func parseOnOff(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "on", "yes", "true":
		return true, nil
	case "off", "no", "false":
		return false, nil
	}
	return false, fmt.Errorf("expected on or off, got %q", v)
}

// parseHostPort reads "name" or "name:port"; DNS-over-TLS defaults to port 853.
func parseHostPort(v string) (string, uint16, error) {
	host, portStr, err := net.SplitHostPort(v)
	if err != nil {
		return v, 853, nil
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil || port == 0 {
		return "", 0, fmt.Errorf("invalid port %q", portStr)
	}
	return host, uint16(port), nil
}
