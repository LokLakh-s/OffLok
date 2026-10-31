// Package daemon is offlokd's core: it runs Unbound behind OffLok's DNS front (which blocks what the lists
// block), points the system DNS at the front, follows the network and falls back when the root servers are out of
// reach, and gives the original DNS back whenever something fails.
package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"

	"github.com/loklakh-s/offlok/internal/blocklist"
	"github.com/loklakh-s/offlok/internal/front"
	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/netinfo"
	"github.com/loklakh-s/offlok/internal/probe"
	"github.com/loklakh-s/offlok/internal/root"
	"github.com/loklakh-s/offlok/internal/settings"
	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/unbound"
	"github.com/loklakh-s/offlok/internal/update"
)

const (
	tick          = 5 * time.Second
	probeDirect   = time.Minute      // how often the root is checked while it answers
	probeFallback = 15 * time.Second // and while it does not
	retryAfter    = time.Minute      // wait after a failure before trying again
	dotRefresh    = 24 * time.Hour
	signinFor     = 5 * time.Minute // strict mode: how long "Sign in to this network" lets the network's DNS through
)

// Paths are where offlokd keeps its files.
type Paths struct {
	Settings string // the user's choices
	State    string // directory for the system DNS backends' saved originals
	Unbound  string // Unbound's configuration directory; must be readable by Unbound under AppArmor and SELinux
}

// Daemon is one running offlokd.
type Daemon struct {
	Paths      Paths
	Backend    sysdns.Backend
	UnboundBin string
	// Engine makes the Unbound that reads the configuration file conf and answers at addr; nil: the unbound program
	// at UnboundBin. UnboundVersion is then the engine's version.
	Engine         func(conf string, addr netip.AddrPort, log func(line string)) unbound.Engine
	UnboundVersion string
	UnboundUser    string // account Unbound drops to; empty keeps root (tests only)
	CABundle       string
	Port           uint16 // the front's, on the loopback: 53
	UnboundPort    uint16 // Unbound's usual port, on 127.0.0.1 only, which the front alone asks
	// UnboundSpare are the ports Unbound takes when the usual one is not free: Windows reserves port ranges for
	// Hyper-V, WSL and Docker at boot, anywhere between 49152 and 65535 by default.
	UnboundSpare []uint16
	Lists        *blocklist.Store
	Version      string   // this offlokd's, which updates compare with
	Updater      *Updater // nil: no updates (development builds, tests)
	RootServers  []netip.AddrPort
	Log          *log.Logger
	Debug        bool // log every probe (never a looked-up name)
	// Tick is how often the loop looks at the network without being told; 0: every 5 seconds. A phone's is longer:
	// the app tells the core of every network change.
	Tick time.Duration
	// Changed, when set, hears of every change of state (on, fallback, off, failed...): Android's notification and
	// Quick Settings tile follow it.
	Changed func(Status)
	// Authorize asks whether a peer who is not root may change settings (polkit on Linux, with a password prompt in
	// its session); nil refuses.
	Authorize func(ipc.Peer) (bool, error)

	mu         sync.Mutex
	settings   settings.Settings
	status     Status
	changed    bool      // settings changed since the loop last looked: act now
	signin     time.Time // until when strict mode lets the network's DNS through
	pause      time.Time // until when the blocking is paused
	trusted    *blocklist.Set
	wake       chan struct{}
	listWake   chan struct{}
	updateWake chan struct{}
	upd        UpdateStatus
	release    *update.Release // the newer release found, signed

	// Owned by the loop goroutine.
	ub        unbound.Engine
	conf      []byte
	front     *front.Server
	serving   atomic.Pointer[front.Server] // the front, for Answer
	listen    []netip.Addr                 // the front's
	ubPort    uint16                       // the port Unbound listens on: UnboundPort, or a spare one
	applied   bool
	rootOK    bool
	rootFails int
	dotOK     bool
	nextProbe time.Time
	retryAt   time.Time
	netPrint  string
	network   sysdns.Network
	ipv6      bool
}

// Status is what offlokd reports.
type Status struct {
	State     State             `json:"state"`
	Error     string            `json:"error,omitempty"`
	ErrorCode string            `json:"error_code,omitempty"` // port-taken, unbound, self-test, system-dns, config
	Warning   string            `json:"warning,omitempty"`    // the system's: android-private-dns, android-other-vpn
	Signin    *time.Time        `json:"signin_until,omitempty"`
	Blocking  BlockingStatus    `json:"blocking"`
	Version   string            `json:"version,omitempty"` // offlokd's
	Platform  string            `json:"platform"`          // the window leaves out what a platform does not have
	Update    UpdateStatus      `json:"update"`
	Backend   string            `json:"backend"`
	Unbound   string            `json:"unbound"`
	Network   sysdns.Network    `json:"network"`
	Settings  settings.Settings `json:"settings"`
	Since     time.Time         `json:"since"`
	CheckedAt time.Time         `json:"checked_at"`
}

// DefaultRootServers are the root servers on port 53.
func DefaultRootServers() []netip.AddrPort {
	out := make([]netip.AddrPort, len(root.Servers))
	for i, a := range root.Servers {
		out[i] = netip.AddrPortFrom(a, 53)
	}
	return out
}

func (d *Daemon) keysDir() string  { return filepath.Join(d.Paths.Unbound, "keys") }
func (d *Daemon) confPath() string { return filepath.Join(d.Paths.Unbound, "unbound.conf") }

// addr is the front's IPv4 address: what the system asks, what the self-test asks.
func (d *Daemon) addr() netip.AddrPort {
	return netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), d.Port)
}

func (d *Daemon) unboundAddr() netip.AddrPort {
	port := d.ubPort
	if port == 0 {
		port = d.UnboundPort
	}
	return netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port)
}

// pickUnboundPort returns the first of Unbound's ports free for both UDP and TCP, the usual one first; none free,
// the usual one, which runUnbound then reports as taken.
func (d *Daemon) pickUnboundPort() uint16 {
	lo := netip.MustParseAddr("127.0.0.1")
	for _, p := range append([]uint16{d.UnboundPort}, d.UnboundSpare...) {
		if portFree([]netip.Addr{lo}, p) != nil {
			continue
		}
		l, err := net.Listen("tcp", netip.AddrPortFrom(lo, p).String())
		if err != nil {
			continue
		}
		l.Close()
		if p != d.UnboundPort {
			d.Log.Printf("unbound: port %d is taken or reserved, using %d", d.UnboundPort, p)
		}
		return p
	}
	return d.UnboundPort
}

// Init loads the settings and prepares Unbound's directories and trust anchor.
func (d *Daemon) Init() error {
	s, err := settings.Load(d.Paths.Settings)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.settings = s
	d.status = Status{State: Starting, Backend: d.Backend.Name(), Settings: s, Since: time.Now()}
	d.wake, d.listWake, d.updateWake = make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1)
	d.trusted = blocklist.NewSet(slices.Clone(s.Trusted))
	d.mu.Unlock()
	if d.Engine != nil {
		d.status.Unbound = d.UnboundVersion
	} else if v, err := unbound.Version(d.UnboundBin); err == nil {
		d.status.Unbound = v
	}
	if err := os.MkdirAll(d.Paths.State, 0o755); err != nil {
		return err
	}
	// The configuration stays root's; only the keys directory belongs to Unbound, which rewrites the trust
	// anchor there on key rollovers.
	if err := os.MkdirAll(d.keysDir(), 0o755); err != nil {
		return err
	}
	anchor := filepath.Join(d.keysDir(), "root.key")
	if _, err := os.Stat(anchor); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(anchor, []byte(root.TrustAnchors), 0o644); err != nil {
			return err
		}
	}
	if d.UnboundUser != "" {
		u, err := user.Lookup(d.UnboundUser)
		if err != nil {
			return fmt.Errorf("unbound's account: %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		for _, p := range []string{d.keysDir(), anchor} {
			if err := os.Chown(p, uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run drives OffLok until ctx ends, then gives the system DNS back and stops Unbound.
func (d *Daemon) Run(ctx context.Context) {
	every := d.Tick
	if every == 0 {
		every = tick
	}
	t := time.NewTicker(every)
	defer t.Stop()
	defer d.turnOff()
	if d.Lists != nil {
		go d.listsLoop(ctx)
	}
	if d.Updater != nil {
		go d.updatesLoop(ctx)
	}
	for {
		d.step(ctx)
		var exited <-chan struct{}
		if d.ub != nil {
			exited = d.ub.Exited()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-d.wake:
		case <-exited:
		}
	}
}

// Wake makes the loop look at the network at once: the system told of a change (Android's app does).
func (d *Daemon) Wake() { d.poke() }

func (d *Daemon) poke() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Status returns the current status.
func (d *Daemon) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.status
	s.Settings = d.settings.Clone()
	s.Version, s.Update, s.Platform = d.Version, d.upd, runtime.GOOS
	s.Blocking.Categories = blocklist.Categories
	if w, ok := d.Backend.(sysdns.Warner); ok {
		s.Warning = w.Warning()
	}
	if !s.Settings.Updates && s.Update.State == "" {
		s.Update = UpdateStatus{} // turned off: nothing to show
	}
	if time.Now().Before(d.signin) {
		until := d.signin
		s.Signin = &until
	}
	if time.Now().Before(d.pause) {
		until := d.pause
		s.Blocking.PausedUntil = &until
	}
	if d.Lists != nil {
		s.Blocking.Lists = d.Lists.Info(selection(d.settings))
	}
	return s
}

// Signin lets the network's DNS through for a few minutes in strict mode, to sign in to a captive portal.
func (d *Daemon) Signin() error {
	d.mu.Lock()
	if !d.settings.Strict {
		d.mu.Unlock()
		return errors.New("only strict mode keeps the network's DNS out")
	}
	d.signin, d.changed = time.Now().Add(signinFor), true
	d.mu.Unlock()
	d.Log.Print("strict mode: the network's DNS let through for 5 minutes")
	d.poke()
	return nil
}

func (d *Daemon) signinActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Now().Before(d.signin)
}

// currentSettings returns the settings, and whether they changed since the last call.
func (d *Daemon) currentSettings() (settings.Settings, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	changed := d.changed
	d.changed = false
	return d.settings.Clone(), changed
}

// Update changes the settings, saves them and lets the loop act on them.
func (d *Daemon) Update(change func(*settings.Settings) error) error {
	d.mu.Lock()
	s := d.settings.Clone()
	if err := change(&s); err != nil {
		d.mu.Unlock()
		return err
	}
	if err := settings.Save(d.Paths.Settings, s); err != nil {
		d.mu.Unlock()
		return err
	}
	d.settings, d.changed = s, true
	d.trusted = blocklist.NewSet(slices.Clone(s.Trusted))
	d.mu.Unlock()
	d.poke()
	for _, c := range []chan struct{}{d.listWake, d.updateWake} {
		select {
		case c <- struct{}{}:
		default:
		}
	}
	return nil
}

func (d *Daemon) setStatus(state State, errMsg, errCode string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	changed := d.status.State != state || d.status.ErrorCode != errCode
	if d.status.State != state {
		d.status.Since = time.Now()
		d.Log.Printf("state: %s", state)
	}
	d.status.State, d.status.Error, d.status.ErrorCode = state, errMsg, errCode
	d.status.Network = d.network
	d.status.CheckedAt = time.Now()
	if changed && d.Changed != nil {
		go func() { d.Changed(d.Status()) }()
	}
}

func (d *Daemon) step(ctx context.Context) {
	s, changed := d.currentSettings()
	if changed {
		d.retryAt, d.nextProbe = time.Time{}, time.Time{}
	}
	if !s.Enabled {
		d.turnOff()
		d.setStatus(Off, "", "")
		return
	}
	if time.Now().Before(d.retryAt) {
		return
	}
	if d.ub != nil && !d.ub.Running() {
		d.Log.Print("unbound stopped unexpectedly")
		_ = d.ub.Stop(0) // what it leaves behind: libunbound's sockets
		d.ub, d.conf = nil, nil
	}
	d.observe(ctx, s)
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var dec decision
		if dec, err = d.converge(ctx, s); err == nil {
			d.setStatus(dec.state, "", "")
			if dec.state == Direct {
				d.refreshDoT(ctx, s)
			}
			return
		}
		if !d.rootOK {
			break
		}
		// The root answered the last probe but recursion fails now: try again as if it were out of reach.
		d.rootOK, d.rootFails = false, 2
		d.dotOK = d.checkDoT(ctx, s)
	}
	d.fail(s, err)
}

// observe reads the network and probes the root (and the personal DoT resolver) when due.
func (d *Daemon) observe(ctx context.Context, s settings.Settings) {
	network, err := d.Backend.Network()
	if err != nil {
		d.Log.Printf("read the network's DNS: %v", err)
		network = d.network
	}
	network = usable(network)
	fp := netinfo.Fingerprint()
	if f, ok := d.Backend.(sysdns.Fingerprinter); ok {
		fp = f.Fingerprint()
	}
	ipv6 := netinfo.HasIPv6Route()
	changed := fp != d.netPrint || !network.Equal(d.network) || ipv6 != d.ipv6
	d.netPrint, d.network, d.ipv6 = fp, network, ipv6
	now := time.Now()
	if !changed && now.Before(d.nextProbe) {
		return
	}
	ok := probe.RootReachable(ctx, d.rootServers())
	if d.Debug {
		d.Log.Printf("debug: root reachable=%v, ipv6=%v, network changed=%v, network=%v", ok, ipv6, changed, network)
	}
	switch {
	case ok:
		d.rootOK, d.rootFails = true, 0
	case changed: // a new network: its first answer counts
		d.rootOK, d.rootFails = false, d.rootFails+1
	default: // one lost answer is not an outage
		d.rootFails++
		if d.rootFails >= 2 {
			d.rootOK = false
		}
	}
	d.dotOK = !d.rootOK && d.checkDoT(ctx, s)
	// A lost answer is checked again soon, so that an outage on the same network is confirmed within seconds.
	if ok {
		d.nextProbe = now.Add(probeDirect)
	} else {
		d.nextProbe = now.Add(probeFallback)
	}
}

// rootServers are the root servers over the address families Unbound uses.
func (d *Daemon) rootServers() []netip.AddrPort {
	var out []netip.AddrPort
	for _, s := range d.RootServers {
		if s.Addr().Is4() || d.ipv6 {
			out = append(out, s)
		}
	}
	return out
}

func (d *Daemon) checkDoT(ctx context.Context, s settings.Settings) bool {
	if s.DoT == nil {
		return false
	}
	for _, a := range s.DoT.Addrs {
		if probe.TLS(ctx, netip.AddrPortFrom(a, s.DoT.Port), s.DoT.Name) == nil {
			return true
		}
	}
	return false
}

// converge makes Unbound and the system DNS match the current situation.
func (d *Daemon) converge(ctx context.Context, s settings.Settings) (decision, error) {
	dec := decide(situation{settings: s, rootOK: d.rootOK, dotOK: d.dotOK, signin: d.signinActive(), network: d.network})
	listen := netinfo.Loopback()
	if d.ub == nil {
		d.ubPort = d.pickUnboundPort()
	}
	cfg := unbound.Config{
		Listen:          []netip.Addr{d.unboundAddr().Addr()},
		Port:            d.unboundAddr().Port(),
		Username:        d.UnboundUser,
		Directory:       d.keysDir(),
		TrustAnchorFile: filepath.Join(d.keysDir(), "root.key"),
		CABundle:        d.CABundle,
		Windows:         runtime.GOOS == "windows",
		IPv6:            d.ipv6,
		Mode:            dec.mode,
		Forwarders:      dec.forwarders,
		NetworkDNS:      d.network.Servers,
		LocalZones:      append(unbound.PrivateZones(), d.validDomains()...),
	}
	conf, err := unbound.Render(cfg)
	if err != nil {
		return dec, coded("config", fmt.Errorf("unbound configuration: %w", err))
	}
	if err := d.runUnbound(ctx, conf); err != nil {
		return dec, err
	}
	if err := d.runFront(listen); err != nil {
		return dec, err
	}
	if dec.state != Offline && dec.state != StrictBlocked {
		if err := d.selfTest(ctx, dec.mode != unbound.ForwardNetwork); err != nil {
			return dec, coded("self-test", fmt.Errorf("self-test (%s): %w", dec.state, err))
		}
	}
	if !d.applied {
		d.Log.Printf("pointing the system DNS (%s) at %v", d.Backend.Name(), listen)
	}
	if ok, err := d.Backend.Applied(listen); err != nil || !ok || !d.applied {
		if err := d.Backend.Apply(listen); err != nil {
			return dec, coded("system-dns", fmt.Errorf("set the system DNS: %w", err))
		}
		d.applied = true
	}
	return dec, nil
}

func (d *Daemon) validDomains() []string {
	var out []string
	for _, n := range d.network.Domains {
		if c, err := unbound.CanonicalName(n); err == nil {
			out = append(out, c)
		} else {
			d.Log.Printf("ignoring the network's domain: %v", err)
		}
	}
	return out
}

// runUnbound starts Unbound, or reloads it when its configuration changed.
func (d *Daemon) runUnbound(ctx context.Context, conf []byte) error {
	if d.ub != nil && bytes.Equal(conf, d.conf) {
		return nil
	}
	if err := os.WriteFile(d.confPath(), conf, 0o644); err != nil {
		return coded("config", err)
	}
	if d.ub == nil {
		if err := portFree([]netip.Addr{d.unboundAddr().Addr()}, d.unboundAddr().Port()); err != nil {
			return coded("port-taken", err)
		}
		e := d.newEngine()
		if err := e.Start(); err != nil {
			return coded("unbound", err)
		}
		d.ub = e
	} else if err := d.ub.Reload(); errors.Is(err, unbound.ErrNoReload) {
		_ = d.ub.Stop(5 * time.Second)
		d.ub = nil
		return d.runUnbound(ctx, conf)
	} else if err != nil {
		return coded("unbound", err)
	}
	d.conf = conf
	if err := waitAlive(ctx, d.unboundAddr(), d.ub); err != nil {
		_ = d.ub.Stop(5 * time.Second)
		d.ub, d.conf = nil, nil
		return coded("unbound", err)
	}
	return nil
}

func (d *Daemon) newEngine() unbound.Engine {
	log := func(l string) { d.Log.Print("unbound: " + l) }
	if d.Engine != nil {
		return d.Engine(d.confPath(), d.unboundAddr(), log)
	}
	return &unbound.Process{Binary: d.UnboundBin, ConfPath: d.confPath(), Log: log}
}

func waitAlive(ctx context.Context, addr netip.AddrPort, p unbound.Engine) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := probe.Alive(ctx, addr); err == nil {
			return nil
		} else if time.Now().After(deadline) || !p.Running() {
			return fmt.Errorf("unbound does not answer on %s: %v", addr, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// portFree checks that no other program answers DNS on OffLok's addresses.
func portFree(listen []netip.Addr, port uint16) error {
	for _, a := range listen {
		ap := netip.AddrPortFrom(a, port).String()
		pc, err := net.ListenPacket("udp", ap)
		if err != nil {
			return fmt.Errorf("%s is already used by another program (another resolver, such as unbound.service or dnsmasq?): %v", ap, err)
		}
		pc.Close()
	}
	return nil
}

func (d *Daemon) selfTest(ctx context.Context, requireAD bool) error {
	var err error
	for i := 0; i < 3; i++ {
		if err = probe.Resolver(ctx, d.addr(), requireAD); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

// fail gives the system DNS back, except in strict mode where the user chose no DNS over the network's, and waits
// before trying again.
func (d *Daemon) fail(s settings.Settings, err error) {
	d.Log.Printf("failure: %v", err)
	msg := err.Error()
	// Restore even when this run applied nothing: a crashed offlokd may have left the system pointing at us.
	if !s.Strict {
		if rerr := d.Backend.Restore(); rerr != nil {
			d.Log.Printf("give the system DNS back: %v", rerr)
			msg += "; giving the system DNS back failed too: " + rerr.Error()
		} else if d.applied {
			d.applied = false
			d.Log.Print("system DNS given back")
		}
	}
	d.retryAt = time.Now().Add(retryAfter)
	code := ""
	var ce *codedError
	if errors.As(err, &ce) {
		code = ce.code
	}
	d.setStatus(Failed, msg, code)
}

// codedError carries a short code the apps translate for people; the message stays for the logs and the CLI.
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

func coded(code string, err error) error { return &codedError{code: code, err: err} }

// runFront starts the front on the loopback addresses, again when they changed (IPv6 switched on or off).
func (d *Daemon) runFront(listen []netip.Addr) error {
	if d.front != nil && d.front.Running() && slices.Equal(listen, d.listen) && d.front.Upstream == d.unboundAddr() {
		return nil
	}
	if d.front != nil {
		d.serving.Store(nil)
		d.front.Stop()
	}
	addrs := make([]netip.AddrPort, len(listen))
	for i, a := range listen {
		addrs[i] = netip.AddrPortFrom(a, d.Port)
	}
	f := &front.Server{Listen: addrs, Upstream: d.unboundAddr(), Blocked: d.blocked}
	if err := f.Start(); err != nil {
		d.front = nil
		return coded("port-taken", fmt.Errorf("%v is already used by another program (another resolver, such as unbound.service or dnsmasq?): %v", addrs, err))
	}
	d.front, d.listen = f, listen
	d.serving.Store(f)
	return nil
}

// Answer answers one query as the front does, for a system that hands its queries over instead of asking on the
// loopback (Android's tunnel): a SERVFAIL while the front is down.
func (d *Daemon) Answer(w dns.ResponseWriter, q *dns.Msg) {
	if f := d.serving.Load(); f != nil {
		f.ServeDNS(w, q)
		return
	}
	r := new(dns.Msg)
	r.SetRcode(q, dns.RcodeServerFailure)
	r.RecursionAvailable = true
	_ = w.WriteMsg(r)
}

// turnOff gives the system DNS back, then stops the front and Unbound. Restoring is idempotent and also undoes
// what a crashed offlokd left behind.
func (d *Daemon) turnOff() {
	if err := d.Backend.Restore(); err != nil {
		d.Log.Printf("give the system DNS back: %v", err)
	} else if d.applied {
		d.applied = false
		d.Log.Print("system DNS given back")
	}
	if d.front != nil {
		d.serving.Store(nil)
		d.front.Stop()
		d.front = nil
	}
	if d.ub != nil {
		_ = d.ub.Stop(5 * time.Second)
		d.ub, d.conf = nil, nil
	}
}

// refreshDoT looks the personal resolver's addresses up again once a day, through OffLok itself.
func (d *Daemon) refreshDoT(ctx context.Context, s settings.Settings) {
	if s.DoT == nil || time.Since(s.DoT.ResolvedAt) < dotRefresh {
		return
	}
	addrs, err := probe.Lookup(ctx, d.addr(), s.DoT.Name)
	if err != nil {
		d.Log.Printf("refresh the personal DNS-over-TLS resolver's addresses: %v", err)
		return
	}
	_ = d.Update(func(n *settings.Settings) error {
		if n.DoT != nil && n.DoT.Name == s.DoT.Name {
			n.DoT.Addrs, n.DoT.ResolvedAt = addrs, time.Now()
		}
		return nil
	})
}

// SetDoT sets the personal DNS-over-TLS resolver after checking that it answers. An empty name removes it.
func (d *Daemon) SetDoT(ctx context.Context, name string, port uint16) error {
	if name == "" {
		return d.Update(func(s *settings.Settings) error { s.DoT = nil; return nil })
	}
	canon, err := unbound.CanonicalName(name)
	if err != nil {
		return err
	}
	name = canon[:len(canon)-1]
	resolvers := []netip.AddrPort{d.addr()}
	for _, a := range d.Status().Network.Servers {
		resolvers = append(resolvers, netip.AddrPortFrom(a, 53))
	}
	var addrs []netip.Addr
	for _, r := range resolvers {
		if addrs, err = probe.Lookup(ctx, r, name); err == nil {
			break
		}
	}
	if len(addrs) == 0 {
		return fmt.Errorf("cannot find the address of %s: %v", name, err)
	}
	var tlsErr error
	for _, a := range addrs {
		if tlsErr = probe.TLS(ctx, netip.AddrPortFrom(a, port), name); tlsErr == nil {
			break
		}
	}
	if tlsErr != nil {
		return fmt.Errorf("%s does not answer DNS-over-TLS on port %d: %v", name, port, tlsErr)
	}
	return d.Update(func(s *settings.Settings) error {
		s.DoT = &settings.DoT{Name: name, Port: port, Addrs: addrs, ResolvedAt: time.Now()}
		return nil
	})
}
