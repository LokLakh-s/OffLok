package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/loklakh-s/offlok/internal/update"
)

const (
	updateFirst = 2 * time.Minute // the first check after offlokd starts, once the network has settled
	updateEvery = 24 * time.Hour
	updateLooks = time.Hour // how often the loop looks whether a check is due
)

// Updater is how offlokd updates OffLok on this system.
type Updater struct {
	Source *update.Source
	Dir    string // where downloads go, offlokd's alone
	// Files names the release's files this system needs: the packages installed, or the installer.
	Files func(update.Release) ([]string, error)
	// Install hands the downloaded files to the system's installer and waits for it. The installer outlives
	// offlokd: on success it stops this offlokd and starts the new one, so Install mostly returns its failures.
	Install func(paths []string) error
}

// UpdateStatus is the updates' part of the status.
type UpdateStatus struct {
	Available string     `json:"available,omitempty"` // a newer version, signed by the release key
	Page      string     `json:"page,omitempty"`      // its release notes
	State     string     `json:"state,omitempty"`     // downloading, installing
	Error     string     `json:"error,omitempty"`     // the last check's or install's
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// updatesLoop looks for a new release once a day while the setting is on: the first time a little after start,
// and at once when the setting is turned on.
func (d *Daemon) updatesLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(updateFirst):
	}
	t := time.NewTicker(updateLooks)
	defer t.Stop()
	for {
		if d.updateDue() {
			_ = d.CheckUpdate(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-d.updateWake:
		}
	}
}

func (d *Daemon) updateDue() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.settings.Updates || d.upd.State != "" {
		return false
	}
	return d.upd.CheckedAt == nil || time.Since(*d.upd.CheckedAt) >= updateEvery
}

// CheckUpdate asks for the latest release and remembers it when it is newer than this offlokd.
func (d *Daemon) CheckUpdate(ctx context.Context) error {
	if d.Updater == nil {
		return errors.New("this build of OffLok does not update itself")
	}
	cctx, cancel := context.WithTimeout(ctx, time.Minute)
	r, err := d.Updater.Source.Latest(cctx)
	cancel()
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upd.CheckedAt = &now
	if err != nil {
		d.upd.Error = err.Error()
		d.Log.Printf("update check: %v", err)
		return err
	}
	d.upd.Error = ""
	if !update.Newer(r.Version, d.Version) {
		d.release, d.upd.Available, d.upd.Page = nil, "", ""
		return nil
	}
	if d.upd.Available != r.Version {
		d.Log.Printf("update: OffLok %s is available", r.Version)
	}
	d.release, d.upd.Available, d.upd.Page = &r, r.Version, d.Updater.Source.Page(r.Version)
	return nil
}

// InstallUpdate downloads the newer release found (looking for one first if none is known), checks every file
// against the signed hashes, and hands them to the system's installer. It returns once the download started; the
// status follows it.
func (d *Daemon) InstallUpdate(ctx context.Context) error {
	if d.Updater == nil {
		return errors.New("this build of OffLok does not update itself")
	}
	d.mu.Lock()
	busy, r := d.upd.State != "", d.release
	d.mu.Unlock()
	if busy {
		return errors.New("an update is already under way")
	}
	if r == nil {
		if err := d.CheckUpdate(ctx); err != nil {
			return err
		}
		d.mu.Lock()
		r = d.release
		d.mu.Unlock()
		if r == nil {
			return fmt.Errorf("OffLok %s is the latest version", d.Version)
		}
	}
	names, err := d.Updater.Files(*r)
	if err != nil {
		return err
	}
	d.mu.Lock()
	if d.upd.State != "" {
		d.mu.Unlock()
		return errors.New("an update is already under way")
	}
	d.upd.State, d.upd.Error = "downloading", ""
	d.mu.Unlock()
	d.Log.Printf("update: downloading OffLok %s", r.Version)
	go d.runUpdate(*r, names)
	return nil
}

func (d *Daemon) runUpdate(r update.Release, names []string) {
	err := func() error {
		// What a previous update left goes first: only files checked now are handed over.
		if err := os.RemoveAll(d.Updater.Dir); err != nil {
			return err
		}
		if err := os.MkdirAll(d.Updater.Dir, 0o755); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		paths := make([]string, len(names))
		for i, name := range names {
			p, err := d.Updater.Source.Download(ctx, r, name, d.Updater.Dir)
			if err != nil {
				return err
			}
			paths[i] = p
		}
		d.mu.Lock()
		d.upd.State = "installing"
		d.mu.Unlock()
		d.Log.Printf("update: installing OffLok %s", r.Version)
		return d.Updater.Install(paths)
	}()
	d.mu.Lock()
	d.upd.State, d.upd.Error = "", ""
	if err != nil {
		d.upd.Error = err.Error()
	}
	d.mu.Unlock()
	if err != nil {
		d.Log.Printf("update: %v", err)
	} else {
		d.Log.Printf("update: OffLok %s installed; it runs once offlokd restarts", r.Version)
	}
}
