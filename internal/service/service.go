// Package service runs panenestd as a service of the user's: a systemd user
// unit on Linux, which lingering starts with the machine and keeps running
// with no one logged in, a launchd agent on macOS, which runs while the
// user is logged in, and a task of Task Scheduler on Windows, which does
// too. `panenest daemon install-service` installs it, as a server wants: it
// runs `panenest daemon serve`, which never exits for being idle, with
// PANENEST_SERVICE=1 under systemd and launchd, which tells panenestd a manager
// starts it again as it exits; Task Scheduler does not.
package service

import (
	"errors"
	"os"
	"strings"

	"panenest/internal/proto"
)

// ErrUnsupported: the system runs no services of a user's that PaneNest knows.
var ErrUnsupported = errors.New("panenest runs panenestd as a service with systemd (Linux), launchd (macOS) or Task Scheduler (Windows)")

// Name is the service's name: panenestd, or panenestd-dev for a development
// build (proto.Name), whose panenestd is of its own.
func Name() string { return strings.Replace(proto.Name(), "panenest", "panenestd", 1) }

// Status is how the service stands.
type Status struct {
	Installed bool
	Running   bool
	// File is the unit or the agent's property list; Detail what the
	// system says of it, as systemctl's "active".
	File   string
	Detail string
	// Linger: systemd keeps the user's services running while they are
	// logged out.
	Linger bool
}

// Installed reports whether panenestd runs as a service.
func Installed() bool {
	f := file()
	if f == "" {
		return false
	}
	_, err := os.Stat(f)
	return err == nil
}
