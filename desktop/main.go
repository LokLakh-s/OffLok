// offlok-desktop is OffLok's tray icon and window. It holds no rights of its own: it asks offlokd over the local
// socket, and offlokd has polkit ask for the administrator's password when a setting needs it.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/i18n"
	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var version = "dev" // set at build time

//go:embed all:frontend
var frontend embed.FS

//go:embed icons/*.png
var icons embed.FS

func icon(name string) []byte {
	b, err := icons.ReadFile("icons/tray-" + name + ".png")
	if err != nil {
		panic(err)
	}
	return b
}

// Result is what the window gets for every call (frontend/api.js).
type Result struct {
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	Denied     bool            `json:"denied,omitempty"`
	NotRunning bool            `json:"not_running,omitempty"`
	Status     json.RawMessage `json:"status,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"` // what a command answers beyond the status (check)
}

// App is the Go service the window calls: main.App.Status, main.App.Send and main.App.OpenNotes.
type App struct {
	socket string
}

// notesPrefix is the only place OpenNotes opens: OffLok's release pages.
const notesPrefix = "https://github.com/LokLakh-s/OffLok/releases/tag/v"

// OpenNotes opens the release notes of the update offlokd found, in the browser.
func (a *App) OpenNotes() Result {
	r := a.Status()
	var st daemon.Status
	if !r.OK || json.Unmarshal(r.Status, &st) != nil || !strings.HasPrefix(st.Update.Page, notesPrefix) {
		return Result{Error: "no release notes to open"}
	}
	if err := application.Get().Browser.OpenURL(st.Update.Page); err != nil {
		return Result{Error: err.Error()}
	}
	return Result{OK: true}
}

// Status returns offlokd's status.
func (a *App) Status() Result { return a.call(ipc.Request{Cmd: "status"}) }

// Send runs one of offlokd's commands. It is interactive: on Linux, offlokd may have polkit ask for the password;
// on Windows, a refusal makes the app run the command again elevated, through UAC.
func (a *App) Send(cmd, value string) Result {
	r := a.call(ipc.Request{Cmd: cmd, Value: value, Interactive: true})
	if !r.Denied {
		return r
	}
	switch err := elevate(cmd, value); {
	case err == nil:
		return a.Status()
	case errors.Is(err, errors.ErrUnsupported), errors.Is(err, errCancelled):
		return r
	default:
		r.Error, r.Denied = err.Error(), false
		return r
	}
}

func (a *App) call(req ipc.Request) Result {
	resp, err := ipc.Call(a.socket, req)
	if errors.Is(err, ipc.ErrNotRunning) {
		return Result{NotRunning: true, Error: err.Error()}
	}
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{OK: resp.OK, Error: resp.Error, Denied: resp.Denied, Status: resp.Status, Data: resp.Data}
}

// trayState picks the tray's picture and tooltip for a status: the dot for fallback and error, grey when off.
func trayState(lang string, st *daemon.Status) (string, string) {
	if st == nil {
		return "off", i18n.T(lang, "err.not-running")
	}
	tip := i18n.T(lang, "state."+string(st.State))
	switch st.State {
	case daemon.Direct:
		return "on", tip
	case daemon.FallbackDoT:
		name := ""
		if st.Settings.DoT != nil {
			name = st.Settings.DoT.Name
		}
		return "fallback", i18n.T(lang, "state.fallback-dot", name)
	case daemon.FallbackNetwork, daemon.StrictBlocked:
		return "fallback", tip
	case daemon.Failed:
		return "error", i18n.T(lang, "state.error", st.Error)
	default:
		return "off", tip
	}
}

// replaced reports whether the program's file changed since it started: an update installed a new one.
func replaced(exe string, started os.FileInfo) bool {
	now, err := os.Stat(exe)
	return err == nil && started != nil && (!now.ModTime().Equal(started.ModTime()) || now.Size() != started.Size())
}

// relaunch starts the new program, which waits for this one to end (one instance only), then quits.
func relaunch(app *application.App, exe string, hidden bool) {
	args := []string{"--after", strconv.Itoa(os.Getpid())}
	if hidden {
		args = append(args, "--hidden")
	}
	if err := exec.Command(exe, args...).Start(); err != nil {
		log.Printf("relaunch after the update: %v", err)
		return
	}
	app.Quit()
}

func main() {
	hidden := flag.Bool("hidden", false, "start in the tray, the window closed (the session's autostart)")
	after := flag.Int("after", 0, "wait for this process to end first (the app relaunching itself after an update)")
	flag.Parse()
	if *after > 0 {
		waitExit(*after, 15*time.Second)
	}
	exe, _ := os.Executable()
	started, _ := os.Stat(exe)
	socket := os.Getenv("OFFLOK_SOCKET")
	if socket == "" {
		socket = ipc.DefaultSocket
	}
	lang := i18n.Lang()
	svc := &App{socket: socket}
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	var window *application.WebviewWindow
	var mu sync.Mutex
	app := application.New(application.Options{
		Name:        "OffLok",
		Description: "Your computer as its own DNS resolver",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "org.loklakh.offlok.desktop",
			// Opening OffLok from the menu while the tray runs shows the running one's window.
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				mu.Lock()
				defer mu.Unlock()
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
		Linux: application.LinuxOptions{ProgramName: "offlok"},
		Mac:   application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
	})

	mu.Lock()
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          "offlok",
		Title:         "OffLok",
		Width:         440,
		Height:        720,
		MinWidth:      380,
		MinHeight:     520,
		Hidden:        *hidden,
		URL:           "/",
		HideOnEscape:  true,
		DisableResize: false,
	})
	mu.Unlock()
	// Closing the window keeps the tray: OffLok's service runs on regardless.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	tray := app.SystemTray.New()
	tray.SetIcon(icon("off"))
	tray.SetTooltip("OffLok")
	menu := app.NewMenu()
	menu.Add(i18n.T(lang, "tray.open")).OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.AddSeparator()
	menu.Add(i18n.T(lang, "tray.quit")).OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.AttachWindow(window).WindowOffset(6)

	go func() {
		last := ""
		for {
			var st *daemon.Status
			if r := svc.Status(); r.OK || len(r.Status) > 0 {
				var s daemon.Status
				if json.Unmarshal(r.Status, &s) == nil {
					st = &s
				}
			}
			// offlokd was updated, and this program with it: the new one takes over.
			if st != nil && st.Version != "" && st.Version != version && version != "dev" && replaced(exe, started) {
				relaunch(app, exe, !window.IsVisible())
				return
			}
			pic, tip := trayState(lang, st)
			if pic+tip != last {
				tray.SetIcon(icon(pic))
				tray.SetTooltip(tip)
				last = pic + tip
			}
			time.Sleep(3 * time.Second)
		}
	}()

	log.Printf("offlok-desktop %s", version)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
