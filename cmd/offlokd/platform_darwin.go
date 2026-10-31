package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/settings"
	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/unbound"
)

const (
	launchdLabel = "org.loklakh.offlokd"
	plistPath    = "/Library/LaunchDaemons/" + launchdLabel + ".plist"
)

// Unbound drops to this hidden account after binding its port; install creates it (macOS has no unbound account).
const unboundUser = "_offlok"

var platformUsage = [2]string{" | install [updates=off] | uninstall",
	"  install    start offlokd at boot with launchd, and now; updates=off turns the update check off\n" +
		"  uninstall  stop offlokd and remove it from launchd, give the system its original DNS back, and remove\n" +
		"             OffLok's files and account\n"}

// Unbound's configuration path is quoted in unbound.conf: the space is fine.
func defaultStateDir() string   { return "/Library/Application Support/OffLok" }
func defaultUnboundDir() string { return "/Library/Application Support/OffLok/unbound" }

func checkPrivileges() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("offlokd must run as root: it sets the system DNS")
	}
	return nil
}

// findUnbound: the .pkg puts its Unbound next to offlokd; otherwise Homebrew's.
func findUnbound() (string, error) {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "unbound", "unbound")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return unbound.FindBinary()
}

// macOS has no polkit: an administrator is root (sudo); the app asks macOS for an administrator itself.
var authorize func(ipc.Peer) (bool, error)

// caBundle is the CA certificates macOS keeps for OpenSSL, used to check DNS-over-TLS servers.
func caBundle() string { return "/etc/ssl/cert.pem" }

// runService runs under launchd, which stops it with SIGTERM; its log goes to offlokd.log. By hand (tests), to the
// terminal.
func runService(stateDir, unboundDir, socket string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if os.Getppid() == 1 {
		logger = openLog(stateDir)
	}
	return serve(ctx, logger, stateDir, unboundDir, socket)
}

func platformCommand(cmd, stateDir string, args []string) bool {
	var err error
	switch cmd {
	case "install":
		err = install(stateDir, args)
	case "uninstall":
		err = uninstall(stateDir)
	default:
		return false
	}
	if err != nil {
		log.Fatal(err)
	}
	return true
}

func launchctl(args ...string) error {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

// launchdPlist starts offlokd at boot, as root, and again 2 seconds after it stops: as Windows's service manager
// does. What it prints before its log is open (a crash) goes to offlokd.err.
func launchdPlist(exe, stateDir string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>2</integer>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, launchdLabel, html.EscapeString(exe), html.EscapeString(filepath.Join(stateDir, "offlokd.err")))
	return b.Bytes()
}

// install creates Unbound's account, then has launchd start offlokd at boot and now. Run again (an upgrade), it
// restarts offlokd on the new program. "updates=off" turns the update check off first.
func install(stateDir string, args []string) error {
	if err := checkPrivileges(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	for _, a := range args {
		if a != "updates=off" {
			return fmt.Errorf("install: unknown argument %q", a)
		}
		path := filepath.Join(stateDir, "settings.json")
		s, err := settings.Load(path)
		if err != nil {
			return err
		}
		s.Updates = false
		if err := settings.Save(path, s); err != nil {
			return err
		}
	}
	if err := createAccount(unboundUser, "OffLok's Unbound"); err != nil {
		return fmt.Errorf("unbound's account: %w", err)
	}
	if err := os.WriteFile(plistPath, launchdPlist(exe, stateDir), 0o644); err != nil {
		return err
	}
	_ = launchctl("bootout", "system/"+launchdLabel) // the previous version's, on an upgrade
	// The previous offlokd gives the DNS back before it ends; launchd takes the job again only once it ended.
	for i := 0; i < 30 && launchctl("print", "system/"+launchdLabel) == nil; i++ {
		time.Sleep(time.Second)
	}
	_ = launchctl("enable", "system/"+launchdLabel)
	if err := launchctl("bootstrap", "system", plistPath); err != nil {
		return err
	}
	return nil
}

// uninstall stops offlokd (which gives the DNS back), takes it out of launchd, gives the DNS back again in case it
// had died, then removes OffLok's files and Unbound's account. The programs are the package's or Homebrew's to
// remove.
func uninstall(stateDir string) error {
	if err := checkPrivileges(); err != nil {
		return err
	}
	_ = launchctl("bootout", "system/"+launchdLabel)
	_ = launchctl("bootout", "system/"+updateLabel) // the last update's job, ended; its file goes with stateDir
	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := sysdns.RestoreAll(stateDir); err != nil {
		return fmt.Errorf("give the DNS back (OffLok's files are kept, run uninstall again): %w", err)
	}
	return errors.Join(
		os.RemoveAll(stateDir),
		os.RemoveAll(filepath.Dir(ipc.DefaultSocket)),
		deleteAccount(unboundUser),
	)
}

func dscl(args ...string) (string, error) {
	out, err := exec.Command("/usr/bin/dscl", append([]string{"."}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("dscl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// freeSystemID is the highest number below 500 (macOS's system accounts) that no user and no group has yet.
func freeSystemID() (int, error) {
	used := map[int]bool{}
	for _, list := range [][]string{{"-list", "/Users", "UniqueID"}, {"-list", "/Groups", "PrimaryGroupID"}} {
		out, err := dscl(list...)
		if err != nil {
			return 0, err
		}
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) == 2 {
				if id, err := strconv.Atoi(f[1]); err == nil {
					used[id] = true
				}
			}
		}
	}
	for id := 499; id >= 300; id-- {
		if !used[id] {
			return id, nil
		}
	}
	return 0, errors.New("no free system account number between 300 and 499")
}

// createAccount makes a hidden system account and its group, with no shell and no password, as macOS's own
// daemons have (_mdnsresponder, _www…). It does nothing when the account exists.
func createAccount(name, realName string) error {
	if _, err := user.Lookup(name); err == nil {
		return nil
	}
	id, err := freeSystemID()
	if err != nil {
		return err
	}
	ids := strconv.Itoa(id)
	for _, args := range [][]string{
		{"-create", "/Groups/" + name},
		{"-create", "/Groups/" + name, "PrimaryGroupID", ids},
		{"-create", "/Groups/" + name, "RealName", realName},
		{"-create", "/Groups/" + name, "Password", "*"},
		{"-create", "/Users/" + name},
		{"-create", "/Users/" + name, "UniqueID", ids},
		{"-create", "/Users/" + name, "PrimaryGroupID", ids},
		{"-create", "/Users/" + name, "RealName", realName},
		{"-create", "/Users/" + name, "UserShell", "/usr/bin/false"},
		{"-create", "/Users/" + name, "NFSHomeDirectory", "/var/empty"},
		{"-create", "/Users/" + name, "Password", "*"},
		{"-create", "/Users/" + name, "IsHidden", "1"},
	} {
		if _, err := dscl(args...); err != nil {
			return err
		}
	}
	return nil
}

func deleteAccount(name string) error {
	if _, err := user.Lookup(name); err != nil {
		return nil
	}
	_, err := dscl("-delete", "/Users/"+name)
	if _, gerr := dscl("-read", "/Groups/"+name); gerr == nil {
		_, gerr = dscl("-delete", "/Groups/"+name)
		err = errors.Join(err, gerr)
	}
	return err
}
