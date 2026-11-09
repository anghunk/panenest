#!/bin/sh
# Installs the server build of PaneNest for the current user, without root, on
# Linux or macOS: panenestd and the panenest command in ~/.local/lib/panenest, the
# command in ~/.local/bin, and panenestd as a service of the user's (systemd's
# or launchd's), so that PaneNest on another machine shows this one's sessions
# (Settings › Hosts). Windows has install-server.ps1.
#
#   PANENEST_RELEASES=<your-release-mirror> sh install-server.sh --version 0.1.0
#   sh install-server.sh [--version 2.1.0] [--no-service] [--dry-run]
#   sh install-server.sh --uninstall
#
# The release workflow writes the version of its release in place of
# @VERSION@. $PANENEST_RELEASES names a mirror of the releases, as panenest daemon
# update takes it.
set -eu

repo=
version='@VERSION@'
lib="$HOME/.local/lib/panenest"
bin="$HOME/.local/bin"
# The file that tells an install of this script's, which updates itself.
marker=.panenest-server

usage() {
	cat <<EOF
Usage: sh install-server.sh [--version VERSION] [--no-service] [--dry-run]
       sh install-server.sh --uninstall

Installs the server build of PaneNest in $lib, the panenest command in $bin, and
runs panenestd as a service: systemd's on Linux, launchd's on macOS
(--no-service: panenestd starts as PaneNest connects). --uninstall removes them, and leaves your settings and data in
~/.config/panenest.
EOF
}

fail() {
	echo "install-server.sh: $*" >&2
	exit 1
}

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO- "$1"
	else
		fail "downloading PaneNest needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# ours reports whether a path is a link to this install's panenest.
ours() {
	[ -L "$1" ] && [ "$(readlink "$1")" = "$lib/panenest" ]
}

uninstall() {
	if [ -x "$lib/panenest" ]; then
		"$lib/panenest" daemon uninstall-service >/dev/null 2>&1 || true
		"$lib/panenest" daemon stop >/dev/null 2>&1 || true
	fi
	for link in "$bin/panenest" "$HOME/.config/panenest/bin/panenest"; do
		if ours "$link"; then
			rm -f "$link"
		fi
	done
	rm -rf "$lib"
	echo "Removed PaneNest from $lib. Your settings and data stay in ~/.config/panenest."
}

main() {
	service=1
	dry_run=0
	while [ $# -gt 0 ]; do
		case "$1" in
		--version)
			[ $# -ge 2 ] || fail "--version needs a version"
			version="${2#v}"
			shift
			;;
		--version=*)
			version="${1#--version=}"
			version="${version#v}"
			;;
		--no-service) service=0 ;;
		--dry-run) dry_run=1 ;;
		--uninstall)
			uninstall
			return
			;;
		-h | --help)
			usage
			return
			;;
		*)
			usage >&2
			exit 2
			;;
		esac
		shift
	done
	[ -n "$repo" ] || [ -n "${PANENEST_RELEASES:-}" ] || fail "PaneNest has no release source configured; build it from source"
	case "$version" in
	@*) [ -n "$repo" ] || fail "a mirror requires an explicit --version" ;;
	esac

	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "the server build of PaneNest is for Linux, macOS and Windows (install-server.ps1), not $(uname -s)" ;;
	esac
	[ "$(id -u)" != 0 ] || fail "run this as the user whose sessions PaneNest runs, not as root"
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "PaneNest is not made for $(uname -m)" ;;
	esac
	case "$version" in
	@*) # This script, run from the repository: the latest release.
		version="$(fetch "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n 1)"
		[ -n "$version" ] || fail "could not find PaneNest's latest release"
		;;
	esac
	case "$version" in
	*[!0-9A-Za-z.-]* | '') fail "$version is not a version of PaneNest" ;;
	esac

	name="panenest-server-$version-$os-$arch.tar.gz"
	url="${PANENEST_RELEASES:-https://github.com/$repo/releases/download}/v$version/$name"
	work="$(mktemp -d)"
	staging="$lib.new"
	trap 'rm -rf "$work" "$staging"' EXIT
	trap 'exit 1' HUP INT TERM

	echo "Downloading PaneNest $version for $os/$arch"
	fetch "$url" >"$work/$name" || fail "could not download $url"
	fetch "$url.sha256" >"$work/$name.sha256" || fail "could not download $url.sha256"
	want="$(cut -d' ' -f1 <"$work/$name.sha256")"
	[ "$(sha256 "$work/$name")" = "$want" ] || fail "$name is not the release's: its SHA-256 differs"
	tar -tzf "$work/$name" >"$work/names" || fail "$name is not a .tar.gz archive"
	if grep -Eq '^/|(^|/)\.\.(/|$)' "$work/names"; then
		fail "$name holds files outside it"
	fi

	if [ "$dry_run" = 1 ]; then
		echo "would install PaneNest $version in $lib, link $bin/panenest, and run panenestd as a service: $service"
		return
	fi

	upgrade=0
	if [ -x "$lib/panenest" ]; then
		upgrade=1
	fi
	echo "Installing in $lib"
	rm -rf "$staging"
	mkdir -p "$staging"
	tar -xzf "$work/$name" -C "$staging"
	[ -x "$staging/panenest" ] || fail "$name holds no panenest"
	"$staging/panenest" version >/dev/null || fail "the panenest of $name does not run here"
	: >"$staging/$marker"
	# The whole directory at once, so that no file of another version stays.
	rm -rf "$lib.old"
	if [ -d "$lib" ]; then
		mv "$lib" "$lib.old"
	fi
	mkdir -p "$(dirname "$lib")"
	mv "$staging" "$lib"
	rm -rf "$lib.old"

	mkdir -p "$bin"
	if [ -e "$bin/panenest" ] || [ -L "$bin/panenest" ]; then
		if ! ours "$bin/panenest"; then
			echo "Left $bin/panenest as it is, which is not this install's. PaneNest reaches this machine with the address me@host?panenest=$lib/panenest"
		fi
	else
		ln -s "$lib/panenest" "$bin/panenest"
	fi
	case ":$PATH:" in
	*":$bin:"*) ;;
	*) echo "Add $bin to your PATH to run panenest, in your shell's profile: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
	esac

	# The service: systemd's user unit, or launchd's agent, which runs in
	# the session of the user logged in at the Mac.
	if [ "$os" = darwin ]; then
		unit="$HOME/Library/LaunchAgents/sh.panenest.panenestd.plist"
		manager() { launchctl print "gui/$(id -u)" >/dev/null 2>&1; }
	else
		unit="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/panenestd.service"
		manager() { command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; }
	fi
	if [ "$upgrade" = 1 ] && "$lib/panenest" daemon status 2>/dev/null | grep -q '^panenestd [0-9][0-9]* running'; then
		# Never a restart unasked: it would end the programs of the sessions.
		echo "panenestd runs the version of before until it restarts: panenest daemon restart (its sessions start again, the programs in them end)."
	elif [ -f "$unit" ]; then
		: # The service runs this install's panenest as it starts.
	elif [ "$service" = 1 ] && manager; then
		"$lib/panenest" daemon install-service || echo "panenestd does not run as a service: PaneNest starts it as it connects."
	else
		echo "panenestd starts as PaneNest connects. To keep it running with the machine: panenest daemon install-service"
	fi

	echo
	echo "PaneNest $version is installed. In PaneNest on your computer, add this machine in Settings › Hosts:"
	echo "  $(id -un)@$(uname -n)"
	echo "or from its command line: panenest hosts add $(uname -n | cut -d. -f1) $(id -un)@$(uname -n)"
}

# All of it read before any of it runs, as a download cut short could stop
# anywhere.
main "$@"
