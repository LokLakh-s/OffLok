// offlok-desktop is OffLok's tray icon and window. It holds no rights of its own: it asks offlokd over the local
// socket; when a setting needs an administrator, polkit asks for the password on Linux, and the app runs the
// command again elevated on Windows (UAC) and macOS (its password prompt).
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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// elevated runs once Windows's prompt (UAC) is gone, which took the focus: the flyout comes back.
	elevated func()
}

// elevating is set while UAC's prompt is up: losing the focus to it does not close the flyout.
var elevating atomic.Bool

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
// on Windows and macOS, a refusal makes the app run the command again as an administrator, through UAC or macOS's
// password prompt.
func (a *App) Send(cmd, value string) Result {
	r := a.call(ipc.Request{Cmd: cmd, Value: value, Interactive: true})
	if !r.Denied {
		return r
	}
	elevating.Store(true)
	err := elevate(cmd, value)
	elevating.Store(false)
	if a.elevated != nil && !errors.Is(err, errors.ErrUnsupported) {
		a.elevated()
	}
	switch {
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
	// The window's and the taskbar's icon. Wails looks for it in the program's resources as number 3, where
	// go-winres puts it as number 1: given here, it is there everywhere.
	appIcon, err := frontend.ReadFile("frontend/img/offlok-mark.png")
	if err != nil {
		log.Fatal(err)
	}
	// On Windows and macOS, the window is a flyout out of the tray icon (the menu bar's, on a Mac), as their own
	// (network, sound): no frame and no taskbar button, above the other windows, gone when it loses the focus. On
	// Linux, a window of its own.
	flyout := runtime.GOOS == "windows" || runtime.GOOS == "darwin"

	var window *application.WebviewWindow
	var tray *application.SystemTray
	var mu sync.Mutex
	// show opens the window, next to the tray icon when it is a flyout.
	show := func() {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case flyout && tray != nil:
			tray.ShowWindow()
		case window != nil:
			window.Show()
			window.Focus()
		}
	}
	app := application.New(application.Options{
		Name:        "OffLok",
		Description: "Your computer as its own DNS resolver",
		Icon:        appIcon,
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "org.loklakh.offlok.desktop",
			// Opening OffLok from the menu while the tray runs shows the running one's window.
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { show() },
		},
		Linux: application.LinuxOptions{ProgramName: "offlok"},
		Mac:   application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
	})

	options := application.WebviewWindowOptions{
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
	}
	if flyout {
		options.Frameless = true
		options.AlwaysOnTop = true
		options.DisableResize = true
		options.Height = 640
		options.Hidden = true // shown next to the tray icon once the app runs
		options.Windows.HiddenOnTaskbar = true
	}
	mu.Lock()
	window = app.Window.NewWithOptions(options)
	mu.Unlock()
	// Closing the window keeps the tray: OffLok's service runs on regardless.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	// The flyout closes when the focus goes elsewhere, except to UAC's prompt for one of its own settings. The
	// click on the tray icon that took the focus away must not open it again at once.
	var closedAt atomic.Int64
	if flyout {
		window.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) {
			if !elevating.Load() && window.IsVisible() {
				window.Hide()
				closedAt.Store(time.Now().UnixNano())
			}
		})
	}

	mu.Lock()
	tray = app.SystemTray.New()
	mu.Unlock()
	tray.SetIcon(icon("off"))
	tray.SetTooltip("OffLok")
	menu := app.NewMenu()
	menu.Add(i18n.T(lang, "tray.open")).OnClick(func(*application.Context) { show() })
	menu.AddSeparator()
	menu.Add(i18n.T(lang, "tray.quit")).OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.AttachWindow(window).WindowOffset(12)
	tray.OnClick(func() {
		switch {
		case window.IsVisible():
			window.Hide()
		case time.Since(time.Unix(0, closedAt.Load())) < 400*time.Millisecond:
		default:
			show()
		}
	})
	svc.elevated = func() {
		if flyout {
			show()
		}
	}
	if flyout && !*hidden {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { show() })
	}

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
