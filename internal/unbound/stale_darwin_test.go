package unbound

import (
	"slices"
	"testing"
)

func TestStalePIDs(t *testing.T) {
	ps := `    1 /sbin/launchd
  412 /opt/homebrew/sbin/unbound -d -c /Library/Application Support/OffLok/unbound/unbound.conf
  413 /opt/homebrew/sbin/unbound -d -c /opt/homebrew/etc/unbound/unbound.conf
  414 vim /Library/Application Support/OffLok/unbound/unbound.conf
`
	got := stalePIDs(ps, "/opt/homebrew/sbin/unbound", "/Library/Application Support/OffLok/unbound/unbound.conf")
	if !slices.Equal(got, []int{412}) {
		t.Fatalf("got %v, want [412]: only OffLok's own Unbound", got)
	}
}
