package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// PaneNest Dev's panes run its own panenest, not the installed PaneNest's. PATH has
// bin last (environment), and login shells put /usr/local/bin, where the
// installed one is, before it anyway (path_helper, fish's /etc/paths). So
// they put bin first once their startup files have run: fish with -C, zsh
// from a .zshenv of PaneNest's (ZDOTDIR), and a session's command in its
// script (shellArgs).

// binFirst returns the bin that panes' shells put first on PATH, "" for
// none.
func (d *Daemon) binFirst() string {
	if !mygo.IsDev() || runtime.GOOS == "windows" {
		return ""
	}
	return filepath.Join(d.dir, "bin")
}

// zdotdir is where zsh finds PaneNest's .zshenv, next to bin.
func zdotdir(bin string) string { return filepath.Join(filepath.Dir(bin), "zsh") }

// zshEnv is the environment that starts zsh with PaneNest's .zshenv, which
// gives the user's ZDOTDIR back.
func zshEnv(bin string) []string {
	return []string{"ZDOTDIR=" + zdotdir(bin), "PANENEST_ZDOTDIR=" + os.Getenv("ZDOTDIR")}
}

// fishBinFirst is fish's command that puts bin first.
func fishBinFirst(bin string) string { return "fish_add_path -mP " + shellQuote("fish", bin) }

// writeZshenv writes PaneNest's .zshenv: the user's ZDOTDIR and .zshenv, then
// bin first before the first prompt, after .zprofile and .zshrc.
func writeZshenv(bin string) error {
	q := shellQuote("zsh", bin)
	s := `if [[ -n $PANENEST_ZDOTDIR ]]; then ZDOTDIR=$PANENEST_ZDOTDIR; else unset ZDOTDIR; fi
[[ -r ${ZDOTDIR:-$HOME}/.zshenv ]] && builtin source ${ZDOTDIR:-$HOME}/.zshenv
__panenest_bin_first() {
  path=(__PANENEST_BIN_PATH__ ${path:#__PANENEST_BIN_PATH__})
  if [[ -n $PANENEST_NODE_INIT ]]; then eval "$PANENEST_NODE_INIT"; fi
  precmd_functions=(${precmd_functions:#__panenest_bin_first})
  unfunction __panenest_bin_first
}
precmd_functions+=(__panenest_bin_first)
`
	dir := zdotdir(bin)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(strings.ReplaceAll(s, "__PANENEST_BIN_PATH__", q)), 0o644)
}
