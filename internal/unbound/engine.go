package unbound

import "time"

// Engine is Unbound at work under offlokd, answering on 127.0.0.1 at its configuration's port: the unbound program
// on computers (Process), the library inside the app on Android (package libunbound). It never restarts itself:
// offlokd decides.
type Engine interface {
	// Start runs Unbound. It returns once Unbound runs, not once it answers: the caller probes for that.
	Start() error
	// Reload makes Unbound read its configuration again, its cache emptied; ErrNoReload: stop it and start it again.
	Reload() error
	// Running reports whether Unbound runs.
	Running() bool
	// Exited is closed when Unbound stops, and nil before Start.
	Exited() <-chan struct{}
	// Stop ends Unbound, at once after timeout.
	Stop(timeout time.Duration) error
}

var _ Engine = (*Process)(nil)
