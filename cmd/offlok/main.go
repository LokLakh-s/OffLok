// offlok is the command that drives offlokd: status, on and off, strict mode, the personal DNS-over-TLS resolver.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/loklakh-s/offlok/internal/blocklist"
	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/i18n"
	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/settings"
)

var version = "dev" // set at build time

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	lang := i18n.Lang()
	socket := os.Getenv("OFFLOK_SOCKET")
	if socket == "" {
		socket = ipc.DefaultSocket
	}
	interactive := len(args) > 0 && args[0] == "--polkit"
	if interactive {
		args = args[1:]
	}
	req, ok := parse(args)
	// --raw <cmd> [value] sends a request as the tray app built it: how the app runs one command elevated on
	// Windows (UAC), where there is no polkit.
	if len(args) >= 2 && args[0] == "--raw" {
		req, ok = ipc.Request{Cmd: args[1], Value: strings.Join(args[2:], " ")}, true
	}
	req.Interactive = interactive
	switch {
	case !ok:
		fmt.Fprint(stderr, i18n.T(lang, "usage"))
		return 2
	case req.Cmd == "help":
		fmt.Fprint(stdout, i18n.T(lang, "usage"))
		return 0
	case req.Cmd == "version":
		fmt.Fprintln(stdout, version)
		return 0
	}
	resp, err := ipc.Call(socket, req)
	if errors.Is(err, ipc.ErrNotRunning) {
		fmt.Fprintln(stderr, i18n.T(lang, notRunningKey))
		return 3
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if resp.Denied {
		fmt.Fprintln(stderr, i18n.T(lang, deniedKey))
		return 4
	}
	if !resp.OK {
		fmt.Fprintln(stderr, resp.Error)
		return 1
	}
	if req.Cmd == "check" {
		printCheck(stdout, lang, resp.Data)
		return 0
	}
	var st daemon.Status
	if err := json.Unmarshal(resp.Status, &st); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch req.Cmd {
	case "status":
	case "update":
		fmt.Fprintln(stdout, i18n.T(lang, "update.started", st.Update.Available))
		return 0
	default:
		fmt.Fprintln(stdout, i18n.T(lang, "done"))
	}
	printStatus(stdout, lang, st)
	return 0
}

func parse(args []string) (ipc.Request, bool) {
	if len(args) == 0 {
		return ipc.Request{Cmd: "status"}, true
	}
	cmd := strings.ToLower(args[0])
	switch {
	case len(args) == 1 && (cmd == "status" || cmd == "on" || cmd == "off" || cmd == "signin" || cmd == "version" || cmd == "update"):
		return ipc.Request{Cmd: cmd}, true
	case len(args) == 1 && (cmd == "help" || cmd == "-h" || cmd == "--help"):
		return ipc.Request{Cmd: "help"}, true
	case len(args) == 2 && (cmd == "strict" || cmd == "dot" || cmd == "access" || cmd == "trust" || cmd == "untrust" || cmd == "check" || cmd == "updates"):
		return ipc.Request{Cmd: cmd, Value: args[1]}, true
	case cmd == "pause" && len(args) <= 2:
		return ipc.Request{Cmd: cmd, Value: strings.Join(args[1:], "")}, true
	case cmd == "block" && len(args) == 3:
		return ipc.Request{Cmd: cmd, Value: strings.ToLower(args[1]) + "=" + strings.ToLower(args[2])}, true
	case cmd == "list" && len(args) == 3:
		return ipc.Request{Cmd: cmd, Value: strings.ToLower(args[1]) + " " + args[2]}, true
	}
	return ipc.Request{}, false
}

func printCheck(w io.Writer, lang string, data []byte) {
	var c daemon.Check
	if json.Unmarshal(data, &c) != nil {
		return
	}
	switch {
	case c.Match == nil:
		fmt.Fprintln(w, i18n.T(lang, "check.free", c.Name))
	case c.Trusted:
		fmt.Fprintln(w, i18n.T(lang, "check.trusted", c.Name, c.Match.Source))
	case c.Paused:
		fmt.Fprintln(w, i18n.T(lang, "check.paused", c.Name, c.Match.Source))
	default:
		fmt.Fprintln(w, i18n.T(lang, "check.blocked", c.Name, i18n.T(lang, "cat."+string(c.Match.Category)), c.Match.Source, c.Match.Entry))
	}
}

func printStatus(w io.Writer, lang string, st daemon.Status) {
	switch st.State {
	case daemon.FallbackDoT:
		name := ""
		if st.Settings.DoT != nil {
			name = st.Settings.DoT.Name
		}
		fmt.Fprintln(w, i18n.T(lang, "state.fallback-dot", name))
	case daemon.Failed:
		fmt.Fprintln(w, i18n.T(lang, "state.error", st.Error))
		if st.Settings.Strict {
			fmt.Fprintln(w, i18n.T(lang, "error.strict-kept"))
		} else {
			fmt.Fprintln(w, i18n.T(lang, "error.given-back"))
		}
	default:
		fmt.Fprintln(w, i18n.T(lang, "state."+string(st.State)))
	}
	if st.Signin != nil {
		fmt.Fprintln(w, i18n.T(lang, "signin.until", st.Signin.Local().Format("15:04")))
	}
	switch u := st.Update; {
	case u.State == "downloading" || u.State == "installing":
		fmt.Fprintln(w, i18n.T(lang, "update."+u.State, u.Available))
	case u.Available != "":
		fmt.Fprintln(w, i18n.T(lang, updateKey, u.Available))
	}
	onOff := func(b bool) string {
		if b {
			return i18n.T(lang, "value.on")
		}
		return i18n.T(lang, "value.off")
	}
	dot := i18n.T(lang, "value.none")
	if st.Settings.DoT != nil {
		dot = fmt.Sprintf("%s:%d", st.Settings.DoT.Name, st.Settings.DoT.Port)
	}
	access := i18n.T(lang, "value.admins")
	if st.Settings.Access == settings.Everyone {
		access = i18n.T(lang, "value.everyone")
	}
	network := i18n.T(lang, "value.none")
	if len(st.Network.Servers) > 0 {
		addrs := make([]string, len(st.Network.Servers))
		for i, a := range st.Network.Servers {
			addrs[i] = a.String()
		}
		network = strings.Join(addrs, ", ")
	}
	if len(st.Network.Domains) > 0 {
		network += fmt.Sprintf(" (%s%s %s)", i18n.T(lang, "value.domains"), i18n.T(lang, "colon"), strings.Join(st.Network.Domains, ", "))
	}
	var cats []string
	for _, c := range blocklist.Categories {
		if st.Settings.Blocking[string(c)] {
			cats = append(cats, i18n.T(lang, "cat."+string(c)))
		}
	}
	blocking := i18n.T(lang, "value.off")
	if len(cats) > 0 {
		names, failed := 0, 0
		for _, l := range st.Blocking.Lists {
			names += l.Names
			if l.Error != "" {
				failed++
			}
		}
		blocking = i18n.T(lang, "blocking.on", strings.Join(cats, ", "), names)
		if failed > 0 {
			blocking += i18n.T(lang, "blocking.failed", failed)
		}
		if st.Blocking.PausedUntil != nil {
			blocking += i18n.T(lang, "blocking.paused", st.Blocking.PausedUntil.Local().Format("15:04"))
		}
	}
	updates := i18n.T(lang, "value.off")
	if st.Settings.Updates {
		updates = i18n.T(lang, "updates.on")
		if st.Update.Error != "" {
			updates += i18n.T(lang, "updates.last-error", st.Update.Error)
		}
	}
	system := fmt.Sprintf("%s, Unbound %s", st.Backend, st.Unbound)
	if st.Version != "" {
		system = fmt.Sprintf("%s, OffLok %s, Unbound %s", st.Backend, st.Version, st.Unbound)
	}
	for _, row := range [][2]string{
		{i18n.T(lang, "label.blocking"), blocking},
		{i18n.T(lang, "label.trusted"), fmt.Sprint(len(st.Settings.Trusted))},
		{i18n.T(lang, "label.strict"), onOff(st.Settings.Strict)},
		{i18n.T(lang, "label.dot"), dot},
		{i18n.T(lang, "label.access"), access},
		{i18n.T(lang, "label.network"), network},
		{i18n.T(lang, "label.updates"), updates},
		{i18n.T(lang, "label.system"), system},
	} {
		label := row[0] + i18n.T(lang, "colon")
		// Pad by characters, not bytes: French labels have accents.
		fmt.Fprintf(w, "  %s%s %s\n", label, strings.Repeat(" ", max(0, 25-utf8.RuneCountInString(label))), row[1])
	}
}
