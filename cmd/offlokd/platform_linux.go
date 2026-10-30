package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/polkit"
	"github.com/loklakh-s/offlok/internal/unbound"
)

// Unbound drops to this account after binding its port (the distribution's package creates it).
const unboundUser = "unbound"

var platformUsage = [2]string{"", ""}

func defaultStateDir() string { return "/var/lib/offlok" }

// Under /var/lib/unbound: Ubuntu's AppArmor profile for Unbound reads nothing else, and SELinux's labels fit.
func defaultUnboundDir() string { return "/var/lib/unbound/offlok" }

func checkPrivileges() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("offlokd must run as root: it sets the system DNS")
	}
	return nil
}

func findUnbound() (string, error) { return unbound.FindBinary() }

// authorize lets polkit ask a desktop user for the administrator's password.
func authorize(p ipc.Peer) (bool, error) { return polkit.Authorized(p.PID, p.UID) }

// caBundle finds the system's CA certificates, used to check DNS-over-TLS servers.
func caBundle() string {
	for _, p := range []string{
		"/etc/ssl/certs/ca-certificates.crt", // Debian, Ubuntu, Arch
		"/etc/pki/tls/certs/ca-bundle.crt",   // Fedora, RHEL
		"/etc/ssl/ca-bundle.pem",             // openSUSE
		"/etc/ssl/cert.pem",                  // Alpine
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// runService runs under systemd, which stops it with SIGTERM; journald adds the time to the log.
func runService(stateDir, unboundDir, socket string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return serve(ctx, log.New(os.Stderr, "", 0), stateDir, unboundDir, socket)
}

func platformCommand(string, string, []string) bool { return false }
