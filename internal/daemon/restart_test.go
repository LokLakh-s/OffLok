package daemon

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/loklakh-s/offlok/internal/unbound"
)

// noReload plays Unbound where it cannot reload (Windows).
type noReload struct{ stops int }

func (e *noReload) Start() error             { return nil }
func (e *noReload) Reload() error            { return unbound.ErrNoReload }
func (e *noReload) Running() bool            { return e.stops == 0 }
func (e *noReload) Exited() <-chan struct{}  { return nil }
func (e *noReload) Stop(time.Duration) error { e.stops++; return nil }

// A network that changes several times in a row restarts Unbound once it has settled, never in between.
func TestRestartWaitsForTheNetworkToSettle(t *testing.T) {
	e := &noReload{}
	d := &Daemon{ub: e, conf: []byte("a"), UnboundPort: 1, Log: log.New(io.Discard, "", 0)}
	d.Paths.Unbound = t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, conf := range []string{"b", "c", "c", "a", "c"} {
		if err := d.runUnbound(ctx, []byte(conf)); err != nil || e.stops != 0 {
			t.Fatalf("%s: restarted at once (stops %d, %v)", conf, e.stops, err)
		}
	}
	d.pendingAt = time.Now().Add(-restartSettle)
	_ = d.runUnbound(ctx, []byte("c")) // the restart itself fails here: no Unbound answers
	if e.stops != 1 {
		t.Fatalf("a settled configuration must restart Unbound once, stops %d", e.stops)
	}
}

// Unbound's "connect failed" storm (no network, on Windows) is folded into one line a minute, with a count.
func TestUnboundLogFoldsConnectFailures(t *testing.T) {
	var out strings.Builder
	d := &Daemon{Log: log.New(&out, "", 0)}
	l := d.unboundLog()
	for range 500 {
		l("error: udp connect failed: No error for 192.0.2.1 port 53")
	}
	l("info: start of service")
	got := out.String()
	if strings.Count(got, "connect failed") != 1 || !strings.Contains(got, "499 more connection failures") ||
		!strings.Contains(got, "start of service") {
		t.Fatalf("folded log:\n%s", got)
	}
}
