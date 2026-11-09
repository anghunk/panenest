// Package build tells what build of PaneNest runs: its version, and whether
// it is the server build (cmd/panenest-server), which runs panenestd and the CLI
// alone, with no app, and updates itself.
package build

import "github.com/egoist/mygo"

// Server is set by the server build.
var Server bool

// Version is the version of PaneNest that runs: the app's, or the one the
// server build was made of.
func Version() string { return mygo.App.Version() }

// Daemon is what panenestd's code is made of, which the app sets (panenestdBuild)
// and panenestd tells clients (proto.Welcome.Build): an update that leaves it
// as it was needs no restart of panenestd. The server build leaves it empty.
var Daemon string
