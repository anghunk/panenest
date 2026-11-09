// PaneNest-server is the server build of PaneNest: panenestd and the panenest CLI,
// without the app, for a machine whose sessions PaneNest on another one shows
// (Settings › Hosts), as a Linux box with no display. It installs in
// ~/.local with install-server.sh, which runs panenestd as a service, and
// updates itself (panenest daemon update, or the app's Update PaneNest).
//
// Its version comes with the build:
//
//	go build -ldflags "-X main.version=2.1.0 -X github.com/egoist/mygo.production=1" -o panenest ./cmd/panenest-server
package main

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"

	"panenest/internal/build"
	"panenest/internal/cli"
	"panenest/internal/daemon"
)

// version is the version of PaneNest this build is of, set with -ldflags.
var version = "dev"

func main() {
	build.Server = true
	mygo.App.SetVersion(version)
	if len(os.Args) > 1 && os.Args[1] == "__panenestd" {
		if err := daemon.Run(daemon.Options{}); err != nil {
			fmt.Fprintln(os.Stderr, "panenestd:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) < 2 {
		os.Exit(cli.Run([]string{"help"}))
	}
	if !cli.IsCommand(os.Args[1]) {
		fmt.Fprintf(os.Stderr, "panenest: unknown command %q (see panenest help)\n", os.Args[1])
		os.Exit(2)
	}
	os.Exit(cli.Run(os.Args[1:]))
}
