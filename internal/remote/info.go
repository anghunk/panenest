package remote

import (
	"os"
	"os/user"
	"runtime"

	"panenest/internal/build"
	"panenest/internal/proto"
	"panenest/internal/update"
)

// LocalInfo describes this machine to a remote client, which peer names.
func LocalInfo(peer string) proto.HostInfo {
	h := proto.HostInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, Home: proto.Home(),
		PaneNest: build.Version(), Version: proto.Version, Server: build.Server, Peer: peer,
		Updates: build.Server && update.Enabled() && update.Managed(update.InstallDir())}
	h.Hostname, _ = os.Hostname()
	if u, err := user.Current(); err == nil {
		h.User = u.Username
	}
	return h
}
