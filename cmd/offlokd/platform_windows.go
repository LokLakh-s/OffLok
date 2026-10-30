package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/settings"
	"github.com/loklakh-s/offlok/internal/sysdns"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "OffLok"

// Windows has no account to drop to: Unbound runs as offlokd does, SYSTEM.
const unboundUser = ""

var platformUsage = [2]string{" | install [updates=off] | uninstall",
	"  install    register and start the Windows service (the installer runs it); updates=off turns the update\n" +
		"             check off\n" +
		"  uninstall  stop and remove the service, and give the system its original DNS back\n"}

func programData() string {
	if d := os.Getenv("ProgramData"); d != "" {
		return d
	}
	return `C:\ProgramData`
}

func defaultStateDir() string   { return filepath.Join(programData(), "OffLok") }
func defaultUnboundDir() string { return filepath.Join(programData(), "OffLok", "unbound") }

// checkPrivileges: the service runs as SYSTEM; by hand (tests), from an administrator's terminal.
func checkPrivileges() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("offlokd must run as an administrator: it sets the system DNS")
	}
	return nil
}

// findUnbound: the installer puts NLnet Labs' Unbound next to offlokd.exe.
func findUnbound() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	p := filepath.Join(filepath.Dir(exe), "unbound", "unbound.exe")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("unbound is not installed next to offlokd: %w", err)
	}
	return p, nil
}

// Windows has no polkit: an administrator is an elevated caller (UAC); the tray app asks for elevation itself.
var authorize func(ipc.Peer) (bool, error)

// caBundle is empty: Unbound checks DNS-over-TLS servers against Windows' own certificate store (tls-win-cert).
func caBundle() string { return "" }

type service struct {
	stateDir, unboundDir, socket string
	err                          error
}

// Execute is the Windows service: it runs OffLok until the service manager asks it to stop.
func (s *service) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, openLog(s.stateDir), s.stateDir, s.unboundDir, s.socket) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				s.err = <-done
				return false, 0
			}
		case err := <-done:
			// serve gave up (Unbound missing, the pipe taken): the service manager's recovery restarts it.
			cancel()
			s.err = err
			return false, 1
		}
	}
}

func runService(stateDir, unboundDir, socket string) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if err := secureStateDir(stateDir); err != nil {
		return err
	}
	if isService {
		s := &service{stateDir: stateDir, unboundDir: unboundDir, socket: socket}
		if err := svc.Run(serviceName, s); err != nil {
			return err
		}
		return s.err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return serve(ctx, log.New(os.Stderr, "", log.LstdFlags), stateDir, unboundDir, socket)
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

// install registers offlokd as a service started at boot, restarted 2 seconds after a crash, and starts it.
// "updates=off" (the installer's box left unticked) turns the update check off first.
func install(stateDir string, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := secureStateDir(stateDir); err != nil {
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
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		s, err = m.CreateService(serviceName, exe, mgr.Config{
			DisplayName: "OffLok",
			Description: "Your computer as its own DNS resolver: Unbound from the root servers, no third party.",
			StartType:   mgr.StartAutomatic,
		}, "run")
		if err != nil {
			return fmt.Errorf("create the service: %w", err)
		}
	}
	defer s.Close()
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 2 * time.Second}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 24*60*60); err != nil {
		return fmt.Errorf("set the service's recovery: %w", err)
	}
	if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return fmt.Errorf("start the service: %w", err)
	}
	if err := setPath(filepath.Dir(exe), true); err != nil {
		return fmt.Errorf("add OffLok to the PATH: %w", err)
	}
	return nil
}

// uninstall stops the service (which gives the DNS back), removes it, gives the DNS back again in case the service
// had died, and takes OffLok out of the PATH.
func uninstall(stateDir string) error {
	if exe, err := os.Executable(); err == nil {
		if err := setPath(filepath.Dir(exe), false); err != nil {
			log.Printf("take OffLok out of the PATH: %v", err)
		}
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(serviceName); err == nil {
		if st, err := s.Control(svc.Stop); err == nil {
			for i := 0; i < 60 && st.State != svc.Stopped; i++ {
				time.Sleep(500 * time.Millisecond)
				if st, err = s.Query(); err != nil {
					break
				}
			}
		}
		err := s.Delete()
		s.Close()
		if err != nil {
			return fmt.Errorf("remove the service: %w", err)
		}
	}
	return sysdns.RestoreAll(stateDir)
}
