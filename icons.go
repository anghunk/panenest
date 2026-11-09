package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
	"panenest/internal/proto"
)

// The icons are Framework7's, drawn after SF Symbols so that they feel at
// home on macOS, a few of other sets where it has none, and programs'
// logos from Simple Icons, as Iconify serves them: see icons/README.md.
//
//go:embed icons/*.svg
var iconFiles embed.FS

// glyph returns an icon of the set, as "f7-gear-alt".
func glyph(name string) *ui.SVG {
	b, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic("panenest: no icon " + name)
	}
	return ui.MustParseSVG(b)
}

// icon draws an icon of panenest's own, on 16, as a status glyph.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	eyeIcon         = icon(`<path d="M1 8s2.5-4.5 7-4.5S15 8 15 8s-2.5 4.5-7 4.5S1 8 1 8Z"/><circle cx="8" cy="8" r="2"/>`)
	eyeOffIcon      = icon(`<path d="m2 2 12 12M6 3.8a8 8 0 0 1 2-.3C12.5 3.5 15 8 15 8a13 13 0 0 1-2.5 2.9M10 12.2a8 8 0 0 1-2 .3C3.5 12.5 1 8 1 8a13 13 0 0 1 2.5-2.9"/>`)
	snippetCopyIcon = icon(`<rect x="5.5" y="5.5" width="8" height="8" rx="1.5"/><path d="M10.5 5.5v-2A1 1 0 0 0 9.5 2.5h-6a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2"/>`)
	snippetEditIcon = icon(`<path d="m9.5 3.5 3 3M3 10l7.5-7.5a1.4 1.4 0 0 1 2 2L5 12l-3 1 1-3Z"/>`)
	snippetIcon     = icon(`<path d="M5 4 1.5 8 5 12M11 4l3.5 4-3.5 4M9.5 2.5l-3 11"/>`)
	sidebarIcon     = glyph("f7-sidebar-left")
	plusIcon        = glyph("f7-plus")
	chevronIcon     = glyph("f7-chevron-right")
	chevronUpIcon   = glyph("f7-chevron-up")
	chevronDownIcon = glyph("f7-chevron-down")
	folderIcon      = glyph("f7-folder")
	searchIcon      = glyph("f7-search")
	terminalIcon    = glyph("meteor-terminal")
	returnIcon      = glyph("f7-return")
	gearIcon        = glyph("f7-gear-alt")
	sunIcon         = icon(`<circle cx="8" cy="8" r="3"/><path d="M8 1v1M8 14v1M1 8h1M14 8h1M3 3l1 1M12 12l1 1M3 13l1-1M12 4l1-1"/>`)
	moonIcon        = icon(`<path d="M13.5 9.5A6 6 0 0 1 6.5 2a6 6 0 1 0 7 7.5Z"/>`)
	splitRightIcon  = glyph("f7-square-split-2x1")
	splitDownIcon   = glyph("f7-square-split-1x2")
	zoomIcon        = glyph("f7-arrow-up-left-arrow-down-right")
	unzoomIcon      = glyph("f7-arrow-down-right-arrow-up-left")
	closeIcon       = glyph("f7-xmark")
	swapIcon        = glyph("f7-arrow-right-arrow-left")
	branchIcon      = glyph("f7-arrow-branch")
	diffIcon        = glyph("octicon-file-diff-16")
	revertIcon      = glyph("f7-arrow-uturn-left")
	openFileIcon    = glyph("f7-arrow-up-right-square")
	globeIcon       = glyph("f7-globe")
	chevronLeftIcon = glyph("f7-chevron-left")
	reloadIcon      = glyph("f7-arrow-clockwise")
	copyIcon        = glyph("f7-doc-on-doc")
	linkIcon        = glyph("f7-arrow-up-right")
	editIcon        = glyph("f7-pencil")
	archiveIcon     = icon(`<rect x="2" y="3" width="12" height="3" rx="1"/><path d="M3 6v6.5a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V6M6.5 9h3"/>`)
)

// programIcons are the logos of programs a pane may run, by the name panenestd
// gives the foreground program, its agent or its script's runtime.
var programIcons = map[string]*ui.SVG{}

func init() {
	for name, logo := range map[string]string{
		"claude": "claude", "codex": "openai", "gemini": "googlegemini", "opencode": "opencode",
		"amp": "amp", "cursor-agent": "cursor", "copilot": "githubcopilot", "qwen": "qwen",
		"kimi": "kimi", "cline": "cline",
		"node": "nodedotjs", "bun": "bun", "deno": "deno", "python": "python", "python3": "python",
		"vim": "vim", "nvim": "neovim", "git": "git", "docker": "docker",
	} {
		programIcons[name] = glyph("si-" + logo)
	}
}

// programIcon is the logo of the pane's foreground program, as Claude's
// for Claude Code and Node.js's for a script node runs; the terminal's
// while the shell is in the foreground or the program has none; a globe
// for a browser.
func programIcon(p *proto.Pane) *ui.SVG {
	if p.Browser() {
		return globeIcon
	}
	for _, name := range []string{p.Agent, p.Process, p.Runtime} {
		if i := programIcons[name]; i != nil {
			return i
		}
	}
	return terminalIcon
}
