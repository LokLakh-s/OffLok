// Package polkit asks polkit whether a local process may do an administrator's action, letting polkit's agent in
// the user's session ask for the password.
package polkit

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Action is OffLok's one polkit action (packaging/linux/org.loklakh.offlok.policy): changing its settings.
const Action = "org.loklakh.offlok.manage"

type subject struct {
	Kind    string
	Details map[string]dbus.Variant
}

type result struct {
	IsAuthorized bool
	IsChallenge  bool
	Details      map[string]string
}

// Authorized reports whether the process pid of user uid may change OffLok's settings, asking for the
// administrator's password when polkit's rules say so (auth_admin_keep: once, then a few minutes without asking).
func Authorized(pid, uid int) (bool, error) {
	start, err := startTime(pid)
	if err != nil {
		return false, err
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		return false, err
	}
	sub := subject{Kind: "unix-process", Details: map[string]dbus.Variant{
		"pid":        dbus.MakeVariant(uint32(pid)),
		"start-time": dbus.MakeVariant(start),
		"uid":        dbus.MakeVariant(int32(uid)),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	const allowUserInteraction = uint32(1)
	var r result
	err = conn.Object("org.freedesktop.PolicyKit1", "/org/freedesktop/PolicyKit1/Authority").CallWithContext(ctx,
		"org.freedesktop.PolicyKit1.Authority.CheckAuthorization", 0, sub, Action, map[string]string{},
		allowUserInteraction, "").Store(&r)
	if err != nil {
		return false, fmt.Errorf("polkit: %w", err)
	}
	return r.IsAuthorized, nil
}

// startTime is the process's start time in clock ticks since boot (field 22 of /proc/<pid>/stat), which polkit
// uses with the pid so that a recycled pid cannot borrow an authorization.
func startTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// The command name, in parentheses, may hold spaces: count the fields after its closing parenthesis.
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 20 {
		return 0, fmt.Errorf("short /proc/%d/stat", pid)
	}
	return strconv.ParseUint(fields[19], 10, 64)
}
