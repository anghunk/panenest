package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"panenest/internal/proto"
)

// skillGuide teaches a coding agent panenest: the skill `panenest skill setup`
// installs, from this binary, so that it follows panenest's updates.
const skillGuide = `---
name: panenest
description: Drive terminal sessions in PaneNest — start agents and commands in their own sessions or panes, type into them, wait for them, read their screens, and report your own status. Use when running inside PaneNest ($PANENEST_PANE is set) and the task needs another terminal, a long-running process, a parallel agent, or the user's attention.
---

# PaneNest

You run inside PaneNest, a terminal whose sessions keep running in the
background. $PANENEST_SESSION is your session and $PANENEST_PANE your pane; the
` + "`panenest`" + ` command drives them. A target is a session or a pane id (a
prefix works), or a session's name; a session stands for its focused pane.

## Run something beside you

    panenest split --down -- npm run dev          # a pane under yours; prints its id
    panenest new -n review -- claude              # a session of its own
    panenest new -n tests -d ~/code/app -- go test ./...
    panenest split --url localhost:3000           # a browser pane, to show the user a page

## Drive it

    panenest send <target> "some text" --enter    # type, then press Enter
    panenest key <target> ctrl+c                  # enter, escape, up, tab, ctrl+d…
    panenest prompt <target> "run the tests"      # send + Enter, wait, print screen
    panenest wait <target> [--for idle,waiting,error,exited] [--timeout 10m]
    panenest wait <target> --exit                 # until its command ends; exits with its status
    panenest new --worktree <branch> -- claude    # an agent on a new branch, in a worktree of its own
    panenest events [target] [--for waiting,error] [--json]
                                              # a line per change, as it happens
    panenest history [query] [--json]             # agents' past conversations, to resume
    panenest resume <conversation>                # a session resuming one
    panenest read <target> [-n 40] [-a]           # the screen, or with scrollback
    panenest status <target> --json               # status, program, directory
    panenest ls                                   # every session and pane

A pane's status is working, waiting (it needs the user), idle, error or
exited. ` + "`panenest wait`" + ` with no --for waits until it stops working. A
command such as a test run doesn't show as working: wait for it with --exit,
or start it with --wait, which exits with the command's status:

    pane=$(panenest split --down -- go test ./...)
    panenest wait "$pane" --exit || panenest read "$pane" -n 80
    panenest split --down --wait -- make build    # the same, in one step

## Use a web page

A browser pane's page is yours to drive, as a user would, to check what
you built: open it with panenest split --url, then

    panenest browser open localhost:3000/todos     # go to a page, wait for it to load
    panenest browser snapshot                      # roles, names and refs: button "Add" [ref=e5]
    panenest browser click --ref e5                # click what a snapshot showed
    panenest browser type --ref e3 "Buy milk"      # fill a field (--enter submits)
    panenest browser click 'button[type=submit]'   # or name an element by CSS selector
    panenest browser wait --text "Welcome"         # or wait <selector>; --timeout 30s
    panenest browser text [selector]               # the page's text, or an element's
    panenest browser html [selector]
    panenest browser eval 'document.title'         # run JavaScript, print its value
    panenest browser screenshot -o /tmp/page.png   # shown or not, without the focus
    panenest browser back|forward|reload
    panenest browser cookies save state.json       # or load: Playwright's, or cookies.txt

Refs last until the page goes to another; one whose element is gone
fails: take a new snapshot.

With no target, panenest browser drives your session's browser pane; name
one (panenest browser <target> text) when there are several. It needs the app
open and Remote Control on, in Settings › Agents.

## Tell the user

    panenest report working "Migrating the database" --progress 40
    panenest report blocked "Approve the production deploy?" --kind permission
    panenest report done "Deployed to staging"
    panenest notify "Need your API key to continue"

Reports are OSC 7501 sequences: PaneNest shows them in its sidebar, and
notifies the user when you are blocked or done while they look elsewhere.
Report blocked only when you truly cannot go on without them.

## Etiquette

- Close what you started when it is done: ` + "`panenest close <target>`" + `.
- Never type into a pane you did not start unless the user asked.
- Exit status 3 means no such target, 4 that PaneNest is not running.
`

func skillCmd(args []string) error {
	f, err := parse(args, map[string]string{"agent": "agent"}, map[string]string{"dry-run": "dry", "n": "dry"})
	if err != nil {
		return err
	}
	sub := "status"
	if len(f.pos) > 0 {
		sub = f.pos[0]
	}
	agent := strings.ToLower(f.vals["agent"])
	dirs, hooks := skillDirs(agent), hooksFor(agent)
	switch sub {
	case "show":
		fmt.Print(skillGuide)
		return nil
	case "status":
		for _, name := range sortedKeys(dirs) {
			fmt.Printf("%-8s skill  %-9s %s\n", name, skillState(dirs[name]), proto.ShortDir(dirs[name]))
		}
		for _, h := range hooks {
			fmt.Printf("%-8s hook   %-9s %s\n", h.name(), h.state(), proto.ShortDir(h.path()))
		}
		return nil
	case "setup", "remove":
		for _, name := range sortedKeys(dirs) {
			dir := dirs[name]
			switch {
			case f.set["dry"]:
				fmt.Printf("would %s %s (%s)\n", sub, proto.ShortDir(dir), name)
			case sub == "remove":
				if err := os.RemoveAll(dir); err != nil {
					return err
				}
				fmt.Printf("removed %s\n", proto.ShortDir(dir))
			default:
				if err := writeSkill(dir); err != nil {
					return err
				}
				fmt.Printf("installed %s (%s)\n", proto.ShortDir(dir), name)
			}
		}
		for _, h := range hooks {
			switch {
			case f.set["dry"]:
				fmt.Printf("would %s the hook in %s (%s)\n", sub, proto.ShortDir(h.path()), h.name())
			case sub == "remove":
				if err := h.remove(); err != nil {
					return err
				}
				fmt.Printf("removed the hook from %s\n", proto.ShortDir(h.path()))
			default:
				if err := h.install(); err != nil {
					return err
				}
				fmt.Printf("installed the hook in %s (%s)\n", proto.ShortDir(h.path()), h.name())
			}
		}
		if sub == "setup" && !f.set["dry"] && len(hooks) > 0 {
			fmt.Println("\nAgents started from now on in PaneNest learn which session they're in.")
			for _, h := range hooks {
				if note := map[string]string{
					"claude":   "Claude Code runs hooks only in folders you trust.",
					"codex":    "Codex asks you to approve a new hook: run /hooks in it.",
					"opencode": "OpenCode's background service loads plugins as it starts: run `opencode service restart` when no session is busy.",
				}[h.name()]; note != "" {
					fmt.Println(note)
				}
			}
		}
		return nil
	}
	return usageErr("panenest skill setup|show|status|remove [--agent claude|codex|pi|opencode|agents] [--dry-run]")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func skillState(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	switch {
	case err != nil:
		return "missing"
	case string(b) != skillGuide:
		return "outdated"
	}
	return "current"
}

func writeSkill(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillGuide), 0o644)
}

// hooksFor returns the hooks that go with an --agent: Codex's, Pi's and
// OpenCode's with the skill they share.
func hooksFor(agent string) []integration {
	switch agent {
	case "", "claude", "codex", "pi", "opencode", "agents":
		return agentHooks(agent)
	}
	return nil
}

// skillDirs returns where agents read skills: Claude Code's, and the
// shared one of Codex, OpenCode and others.
func skillDirs(agent string) map[string]string {
	home := proto.Home()
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(home, ".claude")
	}
	all := map[string]string{
		"claude": filepath.Join(claude, "skills", "panenest"),
		"agents": filepath.Join(home, ".agents", "skills", "panenest"),
	}
	if agent = strings.ToLower(agent); agent != "" {
		if d, ok := all[agent]; ok {
			return map[string]string{agent: d}
		}
		if agent == "codex" || agent == "opencode" || agent == "pi" {
			return map[string]string{"agents": all["agents"]}
		}
	}
	return all
}

// SkillState is where a skill for agents is, and whether it is current,
// with the hooks that go with it.
type SkillState struct {
	Agent string // "claude", or "agents" (Codex, OpenCode, Pi…)
	Dir   string
	State string // "current", "outdated" (or partly installed) or "missing"
}

// SkillStates returns the state of panenest's skill and hooks for each kind
// of agent.
func SkillStates() []SkillState {
	var out []SkillState
	for _, agent := range []string{"claude", "agents"} {
		dir := skillDirs(agent)[agent]
		states := []string{skillState(dir)}
		for _, h := range hooksFor(agent) {
			states = append(states, h.state())
		}
		st := "outdated"
		switch {
		case !slices.ContainsFunc(states, func(s string) bool { return s != "current" }):
			st = "current"
		case !slices.ContainsFunc(states, func(s string) bool { return s != "missing" }):
			st = "missing"
		}
		out = append(out, SkillState{Agent: agent, Dir: dir, State: st})
	}
	return out
}

// InstallSkill installs, or updates, panenest's skill and hooks for a kind of
// agent.
func InstallSkill(agent string) error {
	dir := skillDirs(agent)[agent]
	if dir == "" {
		return fmt.Errorf("no agent %q", agent)
	}
	if err := writeSkill(dir); err != nil {
		return err
	}
	for _, h := range hooksFor(agent) {
		if err := h.install(); err != nil {
			return err
		}
	}
	return nil
}

// RemoveSkill removes panenest's skill and hooks for a kind of agent.
func RemoveSkill(agent string) error {
	dir := skillDirs(agent)[agent]
	if dir == "" {
		return fmt.Errorf("no agent %q", agent)
	}
	for _, h := range hooksFor(agent) {
		if err := h.remove(); err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}
