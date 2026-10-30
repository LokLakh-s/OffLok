// offlokd is OffLok's service: it runs Unbound behind OffLok's DNS front and points the system DNS at it.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/loklakh-s/offlok/internal/blocklist"
	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/update"
)

var version = "dev" // set at build time

func main() {
	stateDir := flag.String("state-dir", defaultStateDir(), "offlokd's own files")
	unboundDir := flag.String("unbound-dir", defaultUnboundDir(), "Unbound's configuration and keys")
	socket := flag.String("socket", ipc.DefaultSocket, "the local channel the offlok command talks to")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: offlokd [flags] [run | restore | version%s]\n\n"+
			"  run      run the service (the default)\n"+
			"  restore  give the system its original DNS back (the uninstaller runs it)\n"+
			"  version  print the version\n%s\n", platformUsage[0], platformUsage[1])
		flag.PrintDefaults()
	}
	flag.Parse()

	switch flag.Arg(0) {
	case "", "run":
		if err := runService(*stateDir, *unboundDir, *socket); err != nil {
			log.Fatal(err)
		}
	case "restore":
		if err := sysdns.RestoreAll(*stateDir); err != nil {
			log.Fatal(err)
		}
		log.Print("restore: the system has its original DNS")
	case "version":
		fmt.Println(version)
	default:
		if !platformCommand(flag.Arg(0), *stateDir, flag.Args()[1:]) {
			flag.Usage()
			os.Exit(2)
		}
	}
}

// serve runs OffLok until ctx ends.
func serve(ctx context.Context, logger *log.Logger, stateDir, unboundDir, socket string) error {
	if err := checkPrivileges(); err != nil {
		return err
	}
	bin, err := findUnbound()
	if err != nil {
		return err
	}
	userAgent := "OffLok/" + version + " (+https://github.com/LokLakh-s/OffLok)"
	d := &daemon.Daemon{
		Paths: daemon.Paths{
			Settings: filepath.Join(stateDir, "settings.json"),
			State:    stateDir,
			Unbound:  unboundDir,
		},
		Backend:     sysdns.Detect(stateDir),
		UnboundBin:  bin,
		UnboundUser: unboundUser,
		CABundle:    caBundle(),
		Port:        53,
		UnboundPort: 53530,
		// Below 49152, spread out: a range Windows reserves cannot hold them all.
		UnboundSpare: []uint16{10530, 20530, 30530, 40530},
		Lists:        &blocklist.Store{Dir: filepath.Join(stateDir, "lists"), Log: logger, UserAgent: userAgent},
		Version:      version,
		RootServers:  daemon.DefaultRootServers(),
		Log:          logger,
		Debug:        os.Getenv("OFFLOK_DEBUG") == "1",
		Authorize:    authorize,
	}
	// A development build has no release to compare with: it never looks for one.
	if version != "dev" {
		d.Updater = newUpdater(&update.Source{UserAgent: userAgent}, filepath.Join(stateDir, "updates"))
	}
	if err := d.Init(); err != nil {
		return err
	}
	l, err := ipc.Serve(socket, d.Handle)
	if err != nil {
		return err
	}
	defer l.Close()
	logger.Printf("offlokd %s, %s, unbound at %s", version, d.Backend.Name(), bin)
	d.Run(ctx)
	logger.Print("stopped")
	return nil
}
