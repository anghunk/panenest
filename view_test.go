package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"panenest/internal/actions"
	"panenest/internal/proto"
	"panenest/internal/settings"

	"panenest/locales"
)

// testApp is the app with a session of two panes side by side, a | b,
// and no panenestd.
func testApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PANENEST_HOME", home)
	// The configuration too: never the user's.
	t.Setenv("PANENEST_CONFIG", filepath.Join(home, "config"))
	a := newApp()
	var l *proto.Layout
	a.st = proto.State{
		Projects: []proto.Project{{ID: "p1", Name: "demo", Dir: "/tmp"}},
		Sessions: []proto.Session{{
			ID: "s1", Project: "p1", Name: "demo",
			Panes: []proto.Pane{
				{ID: "a", Shell: "fish", Dir: "/tmp", Status: proto.Idle, Progress: -1},
				{ID: "b", Shell: "zsh", Dir: "/tmp", Status: proto.Idle, Progress: -1},
			},
			Layout: l.Split("a", "b", "right", 0),
			Focus:  "a",
		}},
	}
	a.loaded, a.selected = true, "s1"
	a.local().loaded = true
	return a
}

// shot writes the tester's frame to $PANENEST_SHOTS, to look at.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("PANENEST_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func testDropApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	a.local().conn = connReady
	a.terms["a"], a.terms["b"] = &termEntry{}, &termEntry{}
	a.local().cl, _ = projectClient(t, func(method string, _ json.RawMessage) (any, error) {
		return nil, fmt.Errorf("test daemon rejected %s", method)
	})
	return a
}

func TestSidebarReceivesFolderDrops(t *testing.T) {
	a := testDropApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	for _, at := range []ui.Point{{X: 10, Y: 30}, {X: 40, Y: 80}, {X: 40, Y: 120}, {X: 100, Y: 400}, {X: 20, Y: 620}} {
		if !tt.DropFiles(at.X, at.Y, t.TempDir()) {
			t.Errorf("sidebar drop at %v was not received", at)
		}
	}
	if tt.DropFiles(a.prefs.SidebarWidth+3, 400, t.TempDir()) {
		t.Error("sidebar received a drop outside its visible bounds")
	}
	shot(t, tt, "sidebar-folder-drop-light")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "sidebar-folder-drop-dark")
}

func TestSidebarRejectsUnavailableDrops(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*App)
	}{
		{"hidden", func(a *App) { a.prefs.SidebarHidden = true }},
		{"connecting", func(a *App) { a.local().conn = connConnecting }},
		{"disconnected", func(a *App) { a.local().conn = connFailed }},
		{"no client", func(a *App) { a.local().cl = nil }},
		{"palette", func(a *App) { a.pal.open = true }},
		{"folder picker", func(a *App) { a.picker = folderPicker{open: true, host: a.local()} }},
		{"only remote connected", func(a *App) {
			h := &host{name: "remote", conn: connReady, cl: a.local().cl, loaded: true}
			a.hosts = append(a.hosts, h)
			a.owner = map[string]*host{"s1": h, "p1": h, "a": h, "b": h}
			a.local().conn, a.local().cl = connFailed, nil
		}},
		{"confirmation", func(a *App) { a.confirm = &confirmation{open: true, title: "Confirm", action: "Close"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testDropApp(t)
			tt := ui.NewTester(a.view, 1100, 640)
			tt.Frame()
			tc.set(a)
			tt.Frame()
			if tt.DropFiles(30, 400, t.TempDir()) {
				t.Errorf("sidebar received a drop while %s", tc.name)
			}
		})
	}
}

func TestSidebarLocalDropWithRemoteSelected(t *testing.T) {
	a := testDropApp(t)
	h := &host{name: "remote", conn: connFailed, loaded: true}
	a.hosts = append(a.hosts, h)
	a.owner = map[string]*host{"s1": h, "p1": h, "a": h, "b": h}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	if !tt.DropFiles(30, 400, t.TempDir()) {
		t.Error("local sidebar drop was blocked by the selected remote host")
	}
}

func TestSidebarKeepsTerminalDrops(t *testing.T) {
	a := testDropApp(t)
	term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	a.terms["a"] = &termEntry{term: term}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	if !tt.DropFiles(400, 340, filepath.Join(t.TempDir(), "image.png")) {
		t.Error("terminal did not receive a file drop while the sidebar target was active")
	}
}

func TestDragPaneToSide(t *testing.T) {
	a := testDropApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	grip, ok := tt.Find("zsh")
	if !ok {
		t.Fatalf("no icon of pane b; texts %q", tt.Texts())
	}
	// 拖动分屏 b 的图标，将它移到分屏 a 的左侧。
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	ar, _ := tt.Find("fish") // 分屏 a 的图标。
	tx, ty := ar.X+10, float32(340)
	tt.Press(x, y)
	for i := 1; i <= 12; i++ {
		f := float32(i) / 12
		tt.Move(x+(tx-x)*f, y+(ty-y)*f)
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("Split Left") {
		t.Errorf("no preview of a split on the left; texts %q", tt.Texts())
	}
	shot(t, tt, "drag-left")

	// Over a's middle: the panes would swap.
	tt.Move(ar.X+120, 340)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("Swap Panes") {
		t.Errorf("no preview of a swap; texts %q", tt.Texts())
	}
	shot(t, tt, "drag-swap")

	tt.Release(ar.X+120, 340)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if got := a.st.Sessions[0].Layout.Panes(); !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("after the drop, panes %v, want b then a", got)
	}
	if a.focusOf(&a.st.Sessions[0]) != "b" {
		t.Errorf("the pane moved has not the focus")
	}
	shot(t, tt, "dropped")
}

// The sidebar's animation moves the header's title one way only, in
// whole pixels: a title that steps back and forth shakes.
func TestSidebarAnimationMonotonic(t *testing.T) {
	bar := ui.TitleBar{Left: 78, Height: 60}
	for _, width := range []float32{200, 268, 333, 440} {
		prevX := float32(-1)
		prevEdge := float32(-1)
		for i := 0; i <= 20000; i++ {
			ch := chromeFor(float32(i)/20000, width, bar)
			x := ch.sideW + ch.mainPad + ch.headerPad // the title's left, in the window
			edge := ch.sideW + ch.mainPad             // the card's left edge
			for _, v := range []float32{ch.sideW, ch.mainPad, ch.headerPad} {
				if v != float32(int(v)) {
					t.Fatalf("width %v, progress %v: %v is not a whole pixel", width, ch.shown, v)
				}
			}
			if x < prevX {
				t.Fatalf("width %v, step %d: the title moved back, %v to %v", width, i, prevX, x)
			}
			if edge < prevEdge {
				t.Fatalf("width %v, step %d: the card's edge moved back, %v to %v", width, i, prevEdge, edge)
			}
			prevX, prevEdge = x, edge
		}
		if ch := chromeFor(1, width, bar); ch.headerPad != 16 || ch.mainPad != 0 || ch.sideW != width {
			t.Errorf("width %v shown: %+v", width, ch)
		}
		if ch := chromeFor(0, width, bar); ch.sideW != 0 || ch.mainPad != cardGap {
			t.Errorf("width %v hidden: %+v", width, ch)
		}
	}
}

// The sidebar's animation never holds still before it lands: a frame
// that does not move, then one that does, reads as a stutter.
func TestSidebarAnimationNoHolds(t *testing.T) {
	ease := sidebarMotion.Ease
	frames := int(sidebarMotion.Duration / (time.Second / 60))
	prev := float32(-1)
	for i := 0; i <= frames; i++ {
		w := chromeFor(ease(float32(i)/float32(frames)), 268, ui.TitleBar{}).sideW
		if w == prev && w != 268 {
			t.Errorf("frame %d: the sidebar held at %v before landing", i, w)
		}
		prev = w
	}
}

// 隐藏项目列表后，顶部标签栏避让窗口按钮，下方分屏图标保持对齐。
func TestHeaderPadBelowTopLeft(t *testing.T) {
	a := testApp(t)
	a.prefs.SidebarHidden = true
	var l *proto.Layout
	a.st.Sessions[0].Layout = l.Split("a", "b", "down", 0)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	top, ok1 := tt.Find("fish")
	below, ok2 := tt.Find("zsh")
	if !ok1 || !ok2 {
		t.Fatalf("no pane icons; texts %q", tt.Texts())
	}
	if below.Y <= top.Y {
		t.Fatalf("pane b is not under a: %v, %v", top, below)
	}
	if abs32(below.X-top.X) > 1 {
		t.Errorf("pane icons below the tab bar are not aligned: %v, %v", top.X, below.X)
	}
	shot(t, tt, "header-pad-below")
}

// The window moves from the panes' headers and the gaps around them, and
// not from a press on a pane, which focuses it.
func TestWindowDragRegions(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	// 标签栏的空白区域用于拖动窗口，并保留当前分屏焦点。
	b, _ := tt.Find("zsh")
	tt.ClickAt(850, 30)
	tt.Frame()
	if f := a.focusOf(&a.st.Sessions[0]); f != "a" {
		t.Errorf("a press on a header's empty stretch moved the focus to %q", f)
	}
	// Pane b's body: it takes the press, and the focus.
	tt.ClickAt(b.X+200, 400)
	tt.Frame()
	if f := a.focusOf(&a.st.Sessions[0]); f != "b" {
		t.Errorf("a press on pane b left the focus on %q", f)
	}
}

// A selection copied as it ends says so; a copy the user asked for, or
// a program's, does not.
func TestCopyOnSelectToast(t *testing.T) {
	a := testApp(t)
	o := a.termOptions("a", nil)
	o.OnCopy("x", terminal.CopyUser)
	o.OnCopy("x", terminal.CopyProgram)
	if a.pendingToast != "" {
		t.Errorf("toast %q for a copy that shows itself", a.pendingToast)
	}
	o.OnCopy("x", terminal.CopySelect)
	if a.pendingToast != "Copied to clipboard" {
		t.Errorf("toast %q after copying a selection", a.pendingToast)
	}
}

// The sidebar's foot counts what wants a look, each count going to the
// next session of it, and says nothing while nothing does ($PANENEST_SHOTS).
func TestSidebarFooter(t *testing.T) {
	a := testApp(t)
	a.local().conn = connReady
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	for _, txt := range tt.Texts() {
		if strings.Contains(txt, "running") {
			t.Errorf("the foot still says %q", txt)
		}
	}
	base := a.st.Sessions[0]
	add := func(id, name string, st proto.Status, unread bool) {
		s := base
		s.ID, s.Name, s.Status, s.Unread = id, name, st, unread
		a.st.Sessions = append(a.st.Sessions, s)
	}
	add("s2", "deploy", proto.Waiting, true)
	add("s3", "refactor", proto.Working, false)
	add("s4", "tests", proto.Working, false)
	add("s5", "docs", proto.Idle, true)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "sidebar-footer")
	for label, want := range map[string]string{"1 session needs you": "s2", "2 sessions working": "s3", "1 session done, unseen": "s5"} {
		a.selected = "s1"
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Errorf("no %q in the foot: %v", label, err)
			continue
		}
		tt.Frame()
		if a.selected != want {
			t.Errorf("%q went to %s, want %s", label, a.selected, want)
		}
	}
}

// A session's tooltip shows below the pointer where it rested, as macOS
// shows tooltips, not centered over the row ($PANENEST_SHOTS).
func TestSessionTooltipAtPointer(t *testing.T) {
	for _, dx := range []float32{20, 120} {
		a := testApp(t)
		a.st.Sessions[0].Name = "tooltip session"
		a.st.Sessions[0].Process = "claude"
		a.st.Sessions[0].Dir = "/Users/someone/dev/panenest"
		a.st.Sessions[0].Git = &proto.GitInfo{Root: "/Users/someone/dev/panenest", Branch: "fix-login", Files: 3, Added: 40, Removed: 2}
		a.st.Sessions[0].Panes[0].Dir = a.st.Sessions[0].Dir
		a.st.Sessions[0].Panes[0].Git = a.st.Sessions[0].Git
		tt := ui.NewTester(a.view, 1100, 700)
		tt.Frame()
		if tt.HasText("/Users/someone/dev/panenest") || tt.HasText("fix-login") {
			t.Fatal("pane details are visible before hovering")
		}
		r, ok := tt.Find("tooltip session")
		if !ok {
			t.Fatal("no session tab")
		}
		x, y := r.X+dx, r.Y+r.H/2
		tt.Move(x, y)
		tt.Advance(2 * time.Second)
		tt.Frame()
		var tip ui.Rect
		found := false
		for _, txt := range tt.Texts() {
			if strings.HasPrefix(txt, "tooltip session\nclaude · ") {
				if !strings.Contains(txt, a.st.Sessions[0].Dir) || !strings.Contains(txt, gitSummary(a.st.Sessions[0].Git)) {
					t.Fatalf("tooltip is missing the full directory or Git summary: %q", txt)
				}
				tip, found = tt.Find(txt)
			}
		}
		if !found {
			t.Fatalf("dx %g: no tooltip; texts %q", dx, tt.Texts())
		}
		if d := tip.X - x; d < 0 || d > 16 {
			t.Errorf("dx %g: tooltip text at x %g, the pointer at %g", dx, tip.X, x)
		}
		if d := tip.Y - y; d < 12 || d > 36 {
			t.Errorf("dx %g: tooltip text at y %g, the pointer at %g", dx, tip.Y, y)
		}
		shot(t, tt, fmt.Sprintf("session-tooltip-%g", dx))
		grip, ok := tt.Find("fish")
		if !ok {
			t.Fatal("no pane icon")
		}
		tt.Move(grip.X+grip.W/2, grip.Y+grip.H/2)
		tt.Advance(2 * time.Second)
		tt.Frame()
		if !tt.HasText("fish\n" + a.st.Sessions[0].Dir + "\n" + gitSummary(a.st.Sessions[0].Git)) {
			t.Fatalf("pane icon tooltip is missing its details: %q", tt.Texts())
		}
	}
}

// A panenestd of another version of PaneNest gets a chip to restart it, which asks
// first while a program runs, as the restart ends it.
func TestRestartStaleDaemon(t *testing.T) {
	a := testApp(t)
	a.local().conn, a.local().daemonApp = connReady, "1.9.0"
	a.st.Sessions[0].Process = "claude"
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	if err := tt.Click("Restart Service"); err != nil {
		t.Fatalf("no chip to restart panenestd: %v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.confirm == nil || !strings.Contains(a.confirm.detail, "PaneNest 1.9.0") {
		t.Fatalf("no question before the restart: %+v", a.confirm)
	}

	a.confirm, a.local().daemonApp = nil, mygo.App.Version()
	tt.Frame()
	if tt.HasText("Restart Service") {
		t.Error("a chip to restart a panenestd of this version")
	}
}

// idleConn is a Conn that shows only what a test feeds the terminal.
type idleConn struct{ closed chan struct{} }

func (c idleConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, os.ErrClosed
}
func (c idleConn) Write(b []byte) (int, error) { return len(b), nil }
func (c idleConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

// Find searches the pane the user is in, its scrollback too: the bar
// counts the matches as one types, Enter and the buttons go through them,
// and Escape closes it, the highlights going; Find Next opens it again
// with the last query.
func TestFindBar(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	var b strings.Builder
	for i := range 200 {
		if i%50 == 7 {
			b.WriteString("error: build failed\r\n")
		} else {
			b.WriteString("ok " + itoa(i) + "\r\n")
		}
	}
	term.Feed([]byte(b.String()))
	a.terms["a"] = &termEntry{term: term}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	a.run("find.open", nil)
	tt.Frame()
	tt.Type("ERROR")
	for range 50 {
		tt.Frame()
		if s := term.SearchStatus(); !s.Searching && s.Selected > 0 {
			break
		}
	}
	tt.Frame()
	if !tt.HasText("1 of 4") {
		t.Fatalf("no count; texts %q", tt.Texts())
	}
	shot(t, tt, "find")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("2 of 4") {
		t.Errorf("after Enter twice and Shift+Enter; texts %q", tt.Texts())
	}
	if err := tt.Click(a.tip(locales.ActionFindNext(), "find.next")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("3 of 4") {
		t.Errorf("after the button for the next; texts %q", tt.Texts())
	}
	shot(t, tt, "find-scrolled")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if a.find.pane != "" || tt.HasText("3 of 4") || term.SearchStatus().Text != "" {
		t.Errorf("Escape left the bar open or the search on: %+v; texts %q", term.SearchStatus(), tt.Texts())
	}
	a.run("find.next", nil)
	tt.Frame()
	tt.Frame()
	if a.find.pane != "a" || a.find.query != "ERROR" || term.SearchStatus().Text != "ERROR" {
		t.Errorf("Find Next did not search again: %+v, %+v", a.find.query, term.SearchStatus())
	}
}

// Command over the path of an image a pane printed, relative to its
// directory, shows the image beside it, until Command is let go of,
// which lets go of the image too.
func TestImagePeek(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	dir := t.TempDir()
	a.st.Sessions[0].Panes[0].Dir = dir
	f, err := os.Create(filepath.Join(dir, "shot.png"))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 30, 20))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{230, 120, 40, 255}), image.Point{}, draw.Src)
	png.Encode(f, img)
	f.Close()
	term, err := terminal.New(terminal.Options{Conn: idleConn{make(chan struct{})}, OpenLink: func(link string) bool { return a.openLink("a", link) }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("saved shot.png:3 and notes.txt\r\n"))
	a.terms["a"] = &termEntry{term: term}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	b, ok := tt.Find("Terminal")
	if !ok {
		t.Fatalf("no terminal; texts %q", tt.Texts())
	}
	// Along the first row to the path, Command pressed with the pointer
	// still.
find:
	for y := b.Y; y < b.Y+40; y += 4 {
		for x := b.X; x < b.X+b.W/2; x += 3 {
			tt.Move(x, y)
			tt.HoldModifiers(ui.Cmd)
			if link, _, _ := term.HoveredLink(); link != "" {
				break find
			}
		}
	}
	tt.Frame()
	if link, path, _ := term.HoveredLink(); link != "shot.png:3" || !path {
		t.Fatalf("hovered %q, %v; text %q", link, path, term.Text())
	}
	if want := filepath.Join(dir, "shot.png"); a.peek.path != want {
		t.Fatalf("peeks at %q, want %q", a.peek.path, want)
	}
	// The decoded image lands, as the window's update would bring it, and
	// shows in the flyout.
	data, _ := os.ReadFile(a.peek.path)
	a.peek.bm, _ = ui.DecodeBitmap(data)
	tt.Frame()
	ft := ui.NewTester(a.peekView, peekMinW, 20+peekPad+peekCaption)
	ft.Frame()
	if !ft.HasText("shot.png") || !ft.HasText("30 × 20") {
		t.Fatalf("no preview; texts %q", ft.Texts())
	}
	shot(t, ft, "peek")
	tt.HoldModifiers(0)
	tt.Frame()
	if a.peek.bm != nil || a.peek.path != "" {
		t.Errorf("the preview stays once Command is let go of: %+v", a.peek)
	}
}

// A split top to bottom, with one side by side inside it, gives every
// split element a key of its own: MyGo panics on two elements with one key
// under one parent, as when a column split made a Row before its Column.
func TestNestedSplitKeys(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes = append(s.Panes, proto.Pane{ID: "c", Shell: "bash", Dir: "/tmp", Status: proto.Idle, Progress: -1})
	s.Layout = &proto.Layout{Ratio: 0.5, A: &proto.Layout{Pane: "a"},
		B: &proto.Layout{Row: true, Ratio: 0.5, A: &proto.Layout{Pane: "b"}, B: &proto.Layout{Pane: "c"}}}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Frame()
	for _, title := range []string{"fish", "zsh", "bash"} {
		if !tt.HasText(title) {
			t.Errorf("no pane %s; texts %q", title, tt.Texts())
		}
	}
}

// A browser pane shows a toolbar over its page, as its header, with an
// address bar that a blank page asks to type in and Open Location gives
// the keyboard; it attaches no terminal.
func TestBrowserPane(t *testing.T) {
	a := testApp(t)
	s := &a.st.Sessions[0]
	s.Panes[1] = proto.Pane{ID: "b", Dir: "/tmp", URL: proto.BlankURL, Status: proto.Idle, Progress: -1}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Frame()
	for _, text := range []string{locales.BrowserBlank()} {
		if !tt.HasText(text) {
			t.Errorf("no %q; texts %q", text, tt.Texts())
		}
	}
	if a.terms["b"] != nil {
		t.Error("a browser pane attached a terminal")
	}
	e := a.webs["b"]
	if e == nil {
		t.Fatal("no browser for pane b")
	}
	// A blank page's address bar takes the keyboard: what is typed there
	// goes to the page, as a URL.
	tt.Type("localhost:5173")
	tt.Frame()
	if e.address != "localhost:5173" {
		t.Errorf("typed %q into the address bar", e.address)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if e.url != "http://localhost:5173" {
		t.Errorf("went to %q", e.url)
	}

	// Its toolbar is its header: no title over it, page's or other.
	s = &a.st.Sessions[0]
	s.Panes[1].Title = "Vite App"
	tt.Frame()
	if tt.HasText("Vite App") || tt.HasText("browser") || !tt.HasText(locales.BrowserAddress()) {
		t.Errorf("a browser pane shows a title bar; texts %q", tt.Texts())
	}
	// Open Location gives the browser's address bar the keyboard, with
	// what it holds to type over.
	a.setFocus(s, "b")
	a.run("browser.address", nil)
	tt.Frame()
	tt.Type("example.org")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if e.url != "https://example.org" {
		t.Errorf("Open Location went to %q", e.url)
	}
	shot(t, tt, "browser")
}

// An address typed in the palette opens in a browser, first of what it
// finds; words find the browser's actions, but open nothing.
func TestPaletteOpensAddress(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	a.openPalette()
	tt.Frame()
	tt.Type("localhost:5173")
	tt.Frame()
	// The palette's rows follow its label.
	texts := tt.Texts()
	rows := texts[slices.Index(texts, locales.PaletteLabel())+1:]
	if len(rows) < 2 || rows[0] != locales.PaletteOpenInBrowser() || rows[1] != "http://localhost:5173" {
		t.Fatalf("the address is not the first item; rows %q", rows)
	}
	a.pal.query = ""
	tt.Frame()
	tt.Type("web")
	tt.Frame()
	if tt.HasText(locales.PaletteOpenInBrowser()) || !tt.HasText(locales.ActionNewBrowser()) {
		t.Errorf("words: texts %q", tt.Texts())
	}
}

// With focus-follows-mouse, the pointer going into a pane focuses it, but
// a pane that comes under a pointer standing still does not.
func TestFocusFollowsMouse(t *testing.T) {
	a := testApp(t)
	a.set.FocusFollowsMouse = true
	s := &a.st.Sessions[0]
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	ar, _ := tt.Find("fish") // pane a's title
	br, _ := tt.Find("zsh")  // pane b's
	move := func(x float32) {
		tt.Move(x, 340)
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	move(ar.X + 40)
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("over a, focus %q", got)
	}
	move(br.X + 40)
	if got := a.focusOf(s); got != "b" {
		t.Fatalf("into b, focus %q, want b", got)
	}

	// The keyboard takes the focus back to a: b, under the pointer, keeps
	// it from there, as the pointer moves on in it.
	a.setFocus(s, "a")
	tt.Frame()
	move(br.X + 60)
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("moving in b after the keyboard focused a, focus %q", got)
	}

	// The panes swap places under the pointer standing still, and back.
	var l *proto.Layout
	s.Layout = l.Split("b", "a", "right", 0)
	tt.Advance(time.Second)
	tt.Frame()
	s.Layout = l.Split("a", "b", "right", 0)
	tt.Advance(time.Second)
	tt.Frame()
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("b came under the pointer standing still, and took the focus")
	}

	// Off, the pointer leaves the focus be.
	a.set.FocusFollowsMouse = false
	move(ar.X + 40)
	move(br.X + 40)
	if got := a.focusOf(s); got != "a" {
		t.Fatalf("with the setting off, focus %q", got)
	}
}

// writes is a Conn that keeps what the terminal sends to the program.
type writes struct {
	idleConn
	got chan []byte
}

func (w writes) Write(b []byte) (int, error) {
	w.got <- append([]byte(nil), b...)
	return len(b), nil
}

// ⌘⌫, ⌘← and ⌘→ send Control+U, Control+A and Control+E, which clear the
// line and go to its start and end, and only on a Mac, where ⌘ is not the
// terminal's Control.
func TestCommandKeysEditLine(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads libghostty-vt")
	}
	if err := terminal.Load(); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	conn := writes{idleConn{make(chan struct{})}, make(chan []byte, 8)}
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	a.terms["a"] = &termEntry{term: term}
	o := a.termOptions("a", nil)
	for _, k := range []struct {
		key  ui.Key
		want string
	}{{ui.KeyBackspace, "\x15"}, {ui.KeyLeft, "\x01"}, {ui.KeyRight, "\x05"}} {
		if took := o.OnKey(ui.Super, k.key); took != (runtime.GOOS == "darwin") {
			t.Fatalf("⌘+%v taken: %v", k.key, took)
		}
		if runtime.GOOS != "darwin" {
			continue
		}
		select {
		case b := <-conn.got:
			if string(b) != k.want {
				t.Errorf("⌘+%v sent %q, want %q", k.key, b, k.want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("⌘+%v sent nothing", k.key)
		}
	}
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{
		{0, ui.KeyBackspace}, {ui.Super | ui.Shift, ui.KeyBackspace}, {ui.Super, ui.KeyDelete},
		{0, ui.KeyLeft}, {ui.Super | ui.Shift, ui.KeyLeft}, {ui.Super | ui.Alt, ui.KeyRight},
	} {
		if o.OnKey(k.mods, k.key) {
			t.Errorf("%v+%v taken", k.mods, k.key)
		}
	}
}

// 长名称应省略显示，并为关闭按钮保留空间；按住快捷键修饰键也不增加文字。
func TestSessionTabKeepsCloseButton(t *testing.T) {
	for _, name := range []string{"session tab", "Investigate why the session list truncates long generated titles"} {
		a := testApp(t)
		a.prefs.SidebarWidth = minSidebar
		tt := ui.NewTester(a.view, 1100, 640)
		a.st.Sessions[0].Name = name
		a.st.Sessions[0].Since = tt.Now().Add(-3 * time.Hour).UnixMilli()
		tt.Frame()
		tt.HoldModifiers(ui.Cmd)
		tt.Frame()
		tab, ok := tt.Find(name)
		close, ok2 := tt.Find(locales.ActionCloseSession())
		if !ok || !ok2 || close.W < 20 || close.X < tab.X || close.X+close.W > tab.X+tab.W || abs32(close.Y+close.H/2-tab.Y-tab.H/2) > 1 {
			t.Fatalf("%q: close button %v is outside tab %v", name, close, tab)
		}
		if tt.HasText(locales.AgoHours(3)) || tt.HasText(actions.Glyphs("CmdOrCtrl+1")) {
			t.Fatalf("%q: tab still shows time or shortcut text; texts %q", name, tt.Texts())
		}
	}
}

// 会话标签只显示单行名称和关闭按钮，旧版详细模式设置不再增加第二行。
func TestSessionTabsSingleLine(t *testing.T) {
	a := testApp(t)
	a.local().conn = connReady
	a.st.Sessions[0].Name = "session tab"
	a.st.Sessions[0].Dir = "/Users/someone/dev/demo"
	a.st.Sessions[0].Panes[0].Dir = a.st.Sessions[0].Dir
	a.st.Sessions[0].Panes[0].Process = "node"
	a.st.Sessions[0].Panes[0].Title = "preview-pane"
	a.st.Sessions[0].Panes[0].Git = &proto.GitInfo{Root: a.st.Sessions[0].Dir, Branch: "fix-login-redirect-after-oauth", Default: "main", Ahead: 2, Files: 3, Added: 40, Removed: 2}
	base := a.st.Sessions[0]
	add := func(id, name string, st proto.Status, msg string, g *proto.GitInfo) {
		s := base
		s.ID, s.Name, s.Status, s.Message, s.Git = id, name, st, msg, g
		a.st.Sessions = append(a.st.Sessions, s)
	}
	add("s2", "migrate", proto.Working, "Migrating the\ndatabase", nil)
	add("s3", "login", proto.Idle, "", &proto.GitInfo{Root: "/tmp/demo", Branch: "fix-login-redirect-after-oauth", Default: "main", Ahead: 2, Files: 3, Added: 40, Removed: 2})
	a.st.Sessions[1].Progress = 40
	tt := ui.NewTester(a.view, 1100, 700)
	for _, legacy := range []string{"detailed", "compact"} {
		a.set.SidebarRows = legacy
		tt.Frame()
		for _, name := range []string{"session tab", "migrate", "login"} {
			if !tt.HasText(name) {
				t.Errorf("missing tab %q", name)
			}
		}
		texts := strings.Join(tt.Texts(), "\n")
		for _, unwanted := range []string{"Migrating the database", "fix-login-redirect-after-oauth", "/Users/someone/dev/demo", "node"} {
			if strings.Contains(texts, unwanted) {
				t.Errorf("tabs still show detail %q; texts %q", unwanted, tt.Texts())
			}
		}
		if got := countText(tt.Texts(), locales.ActionCloseSession()); got != 1 {
			t.Errorf("got %d close buttons, want 1 for the selected tab", got)
		}
	}
	shot(t, tt, "session-tabs-single-line")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "session-tabs-single-line-dark")
	// 后台标签悬停后显示关闭按钮，移入按钮时仍可点击，标签位置保持不变。
	background, ok := tt.Find("migrate")
	if !ok {
		t.Fatal("未找到后台标签")
	}
	tt.Move(background.X+background.W/2, background.Y+background.H/2)
	tt.Frame()
	if got := countText(tt.Texts(), locales.ActionCloseSession()); got != 2 {
		t.Fatalf("悬停后台标签后有 %d 个关闭按钮，应为 2 个", got)
	}
	if after, _ := tt.Find("migrate"); after != background {
		t.Fatalf("悬停后标签位置发生变化：%v -> %v", background, after)
	}
	shot(t, tt, "session-tabs-hover-close")
	tt.Move(500, 400)
	tt.Frame()
	if got := countText(tt.Texts(), locales.ActionCloseSession()); got != 1 {
		t.Fatalf("移开鼠标后有 %d 个关闭按钮，应为 1 个", got)
	}
	tt.Move(background.X+background.W-18, background.Y+background.H/2)
	tt.Frame()
	tt.ClickAt(background.X+background.W-18, background.Y+background.H/2)
	tt.Frame()
	if a.session("s2") != nil || a.selected != "s1" {
		t.Fatalf("关闭后台标签失败或改变了当前标签：selected=%q", a.selected)
	}
	// 从后台标签查看变更时切换到对应会话，再次选择仍保持变更面板打开。
	for range 2 {
		if err := tt.RightClick("login"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(locales.GitShowChanges()); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.selected != "s3" || a.prefs.Side != sideChanges {
			t.Fatalf("Show Changes selected %q with side %q", a.selected, a.prefs.Side)
		}
	}
}

// 打开项目仅保留标题栏入口，隐藏侧边栏后仍能从标题栏选择项目文件夹。
func TestSidebarOpenProjectButton(t *testing.T) {
	a := testApp(t)
	a.mergeState(a.local(), a.st)
	withHost(t, a, "build")
	cl, stop := projectClient(t, func(method string, _ json.RawMessage) (any, error) {
		if method != "fs.list" {
			return nil, fmt.Errorf("unexpected method %s", method)
		}
		return proto.FSList{Dir: "/tmp"}, nil
	})
	defer stop()
	a.local().cl = cl
	label := a.tip(locales.ActionOpenProject(), "project.open")
	footer := ui.NewTester(func(c *ui.Context) { a.sidebarFooter(c, &lightPalette) }, 268, 36)
	footer.Frame()
	if footer.HasText(label) {
		t.Fatal("sidebar footer still contains Open Project")
	}
	tt := ui.NewTester(a.view, 1100, 700)
	for _, hidden := range []bool{false, true} {
		a.prefs.SidebarHidden = hidden
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		r, ok := tt.Find(label)
		if !ok || r.Y+r.H > topBar || !hidden && countText(tt.Texts(), label) != 1 {
			t.Fatalf("hidden %v: expected one title-bar Open Project button; texts %q", hidden, tt.Texts())
		}
		if hidden {
			_, found := tt.Find(a.tip(locales.SidebarShow(), "sidebar.toggle"))
			if !found {
				t.Fatal("hidden sidebar has no title-bar controls")
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		} else if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !a.picker.open || a.picker.host != a.local() {
			t.Fatalf("hidden %v: title-bar button did not open the folder picker", hidden)
		}
		a.picker = folderPicker{}
	}
}

// TestSidebarButtonsKeepVerticalGeometry 验证侧栏切换时顶部按钮组的高度和纵向位置保持一致。
func TestSidebarButtonsKeepVerticalGeometry(t *testing.T) {
	for _, page := range []string{"session", "empty"} {
		t.Run(page, func(t *testing.T) {
			a := testApp(t)
			if page == "empty" {
				a.st = proto.State{}
				a.selected = ""
			}
			tt := ui.NewTester(a.view, 1100, 700)
			tt.SetTitleBar(ui.TitleBar{Left: 78, Height: topBar})
			tt.Frame()
			initial, ok := tt.Find(a.tip(locales.SidebarHide(), "sidebar.toggle"))
			if !ok {
				t.Fatal("未找到侧栏切换按钮")
			}
			if abs32(initial.Y+initial.H/2-float32(cardGap+tabBarH/2)) > 0.5 {
				t.Fatalf("顶部按钮 %v 未与标签栏垂直居中", initial)
			}
			shot(t, tt, "sidebar-buttons-"+page+"-expanded")
			for _, hidden := range []bool{true, false} {
				a.toggleSidebar()
				tt.Frame()
				for frame := 0; frame < 18; frame++ {
					tt.Advance(time.Second / 60)
					for _, label := range []string{
						a.tip(locales.SidebarHide(), "sidebar.toggle"),
						a.tip(locales.SidebarShow(), "sidebar.toggle"),
					} {
						r, ok := tt.Find(label)
						if ok && r.W > 0 && r.H > 0 && (abs32(r.X-initial.X) > 0.5 || abs32(r.Y-initial.Y) > 0.5 || r.H != initial.H) {
							t.Fatalf("侧栏隐藏 %v，第 %d 帧，按钮 %q：原位置 %v，当前位置 %v", hidden, frame, label, initial, r)
						}
					}
				}
				label := a.tip(locales.SidebarHide(), "sidebar.toggle")
				if hidden {
					label = a.tip(locales.SidebarShow(), "sidebar.toggle")
				}
				r, ok := tt.Find(label)
				if !ok || abs32(r.X-initial.X) > 0.5 || abs32(r.Y-initial.Y) > 0.5 || r.H != initial.H {
					t.Fatalf("侧栏隐藏 %v，按钮 %q：原位置 %v，当前位置 %v，存在 %v", hidden, label, initial, r, ok)
				}
				if hidden {
					shot(t, tt, "sidebar-buttons-"+page+"-collapsed")
				}
			}
		})
	}
}

// 中键可关闭后台标签，移出后松开不会关闭，并且保留运行中会话的关闭确认。
func TestSessionTabMiddleClick(t *testing.T) {
	a := testApp(t)
	other := a.st.Sessions[0]
	other.ID, other.Name = "s2", "background tab"
	a.st.Sessions = append(a.st.Sessions, other)
	var ctx *ui.Context
	tt := ui.NewTester(func(c *ui.Context) { ctx = c; a.view(c) }, 1100, 700)
	tt.Frame()
	input := func(id string, kind ui.InputKind, button int, x float32) bool {
		return a.sessionTabInput(ctx, id, ui.InputEvent{Kind: kind, Button: button, X: x, Y: 10})
	}
	for _, button := range []int{0, 1} {
		if input("s2", ui.InputPointerDown, button, 10) {
			t.Fatalf("tab handler consumed button %d", button)
		}
	}
	if input("s2", ui.InputPointerUp, 2, 10) {
		t.Fatal("middle release without a press was consumed")
	}
	if !input("s2", ui.InputPointerDown, 2, 10) || a.selected != "s1" || a.session("s2") == nil {
		t.Fatal("middle press changed the selected tab or closed it early")
	}
	input("s2", ui.InputPointerUp, 2, 10)
	tt.Frame()
	if a.session("s2") != nil || a.selected != "s1" {
		t.Fatal("middle click did not close the background tab while keeping the selected tab")
	}
	input("s1", ui.InputPointerDown, 2, 10)
	input("s1", ui.InputPointerUp, 2, 300)
	tt.Frame()
	if a.session("s1") == nil {
		t.Fatal("middle release outside the tab closed it")
	}
	a.session("s1").Process = "claude"
	a.set.ConfirmClose = true
	input("s1", ui.InputPointerDown, 2, 10)
	input("s1", ui.InputPointerUp, 2, 10)
	tt.Frame()
	if a.confirm == nil || !a.confirm.open || a.session("s1") == nil {
		t.Fatal("middle click bypassed the session close confirmation")
	}
	a.confirm = nil
	a.set.ConfirmClose = false
	input("s1", ui.InputPointerDown, 2, 10)
	input("s1", ui.InputPointerUp, 2, 10)
	tt.Frame()
	if a.session("s1") != nil || a.selected != "" || a.prefs.Project != "p1" {
		t.Fatal("closing the final tab did not keep the empty project selected")
	}
}

// 项目列表只显示项目，顶部标签负责会话切换，切回项目时恢复上次选择。
func TestProjectSessionTabs(t *testing.T) {
	a := testApp(t)
	a.st.Projects = append(a.st.Projects,
		proto.Project{ID: "p2", Name: "other project", Dir: "/tmp/other"},
		proto.Project{ID: "empty", Name: "empty project", Dir: "/tmp/empty"})
	a.st.Sessions[0].Name = "first tab"
	base := a.st.Sessions[0]
	for _, item := range []struct{ id, project, name string }{
		{"s2", "p1", "second tab"}, {"s3", "p2", "other tab"},
	} {
		s := base
		s.ID, s.Project, s.Name = item.id, item.project, item.name
		a.st.Sessions = append(a.st.Sessions, s)
	}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	first, ok := tt.Find("first tab")
	second, ok2 := tt.Find("second tab")
	pane, ok3 := tt.Find("fish")
	if !ok || !ok2 || !ok3 || first.X < a.prefs.SidebarWidth || second.X <= first.X || first.Y >= pane.Y || tt.HasText("other tab") {
		t.Fatalf("unexpected project/tab layout: first %v, second %v, pane %v; texts %q", first, second, pane, tt.Texts())
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	click("second tab")
	if a.selected != "s2" {
		t.Fatalf("clicking a tab selected %q", a.selected)
	}
	click("other project")
	if a.selected != "s3" || tt.HasText("second tab") || !tt.HasText("other tab") {
		t.Fatalf("switching projects selected %q; texts %q", a.selected, tt.Texts())
	}
	click("demo")
	if a.selected != "s2" {
		t.Fatalf("returning to the project selected %q", a.selected)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if a.selected != "s1" {
		t.Fatalf("next tab selected %q", a.selected)
	}
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if a.selected != "s2" {
		t.Fatalf("second-tab shortcut selected %q", a.selected)
	}
	a.startRename("s2")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("renamed tab")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.session("s2").Name != "renamed tab" || !tt.HasText("renamed tab") {
		t.Fatalf("tab rename failed; texts %q", tt.Texts())
	}
	r, _ := tt.Find("renamed tab")
	tt.ClickAt(r.X+r.W-18, r.Y+r.H/2)
	tt.Frame()
	if a.session("s2") != nil || a.selected != "s1" {
		t.Fatalf("closing a tab selected %q; remaining session %v", a.selected, a.session("s2"))
	}
	click("empty project")
	if a.selected != "" || a.prefs.Project != "empty" || !tt.HasText(locales.EmptyTitle()) || tt.HasText("first tab") {
		t.Fatalf("empty project selection: %q, %q; texts %q", a.selected, a.prefs.Project, tt.Texts())
	}
	a.reselect()
	if a.selected != "" || a.prefs.Project != "empty" {
		t.Fatal("state refresh left the selected empty project")
	}
	restored := newApp()
	if restored.selected != "" || restored.prefs.Project != "empty" || restored.prefs.ProjectSessions["p1"] != "s1" {
		t.Fatalf("project selection was not saved: %+v", restored.prefs)
	}
	click("demo")
	a.closeSession("s1", false)
	tt.Frame()
	if a.selected != "" || a.prefs.Project != "p1" || tt.HasText("other tab") {
		t.Fatalf("closing the final tab left its project: %q, %q", a.selected, a.prefs.Project)
	}
	shot(t, tt, "project-tabs-empty")
}

// TestProjectRenameTextVisible 验证项目重命名时，文字在浅色和深色背景上均可辨认。
func TestProjectRenameTextVisible(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := map[bool]string{false: "light", true: "dark"}[dark]
		t.Run(name, func(t *testing.T) {
			a := testApp(t)
			a.st.Projects[0].Name = "MMMMMMMM"
			tt := ui.NewTester(a.view, 1100, 640)
			tt.SetDark(dark)
			tt.Frame()
			tt.Advance(time.Second)
			r, ok := tt.Find("MMMMMMMM")
			if !ok {
				t.Fatalf("project row missing; texts %q", tt.Texts())
			}
			if err := tt.RightClick("MMMMMMMM"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(locales.SessionRename()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.renamingProject != "p1" {
				t.Fatal("project rename input did not open")
			}
			shot(t, tt, "project-rename-"+name)
			img, ink := tt.Image(), 0
			// 只检查文字区域，避开行边框、输入光标和其他控件。
			for y := int(r.Y) + 8; y < int(r.Y+r.H)-8; y++ {
				for x := int(r.X) + rowInset; x < int(r.X)+rowInset+65; x++ {
					p := img.RGBAAt(x, y)
					if (!dark && max(p.R, p.G, p.B) < 128) || (dark && min(p.R, p.G, p.B) > 160) {
						ink++
					}
				}
			}
			if ink < 40 {
				t.Errorf("rename text is not visible on the %s background: %d contrasting pixels", name, ink)
			}
		})
	}
}

// 多个标签超出宽度后可以滚动，快捷键选中的标签会自动进入可见范围。
func TestProjectTabsOverflow(t *testing.T) {
	a := testApp(t)
	base := a.st.Sessions[0]
	for i := 2; i <= 9; i++ {
		s := base
		s.ID, s.Name = fmt.Sprintf("s%d", i), fmt.Sprintf("session tab %d", i)
		a.st.Sessions = append(a.st.Sessions, s)
	}
	tt := ui.NewTester(a.view, 800, 600)
	tt.Frame()
	tt.Key(ui.Cmd, ui.Key9)
	tt.Frame()
	r, ok := tt.Find("session tab 9")
	if a.selected != "s9" || !ok || r.X < a.prefs.SidebarWidth || r.X+r.W > 800-cardGap {
		t.Fatalf("selected tab is outside the viewport: %q, %v", a.selected, r)
	}
	shot(t, tt, "project-tabs-overflow")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "project-tabs-overflow-dark")
}

// TestSessionTabDragOrder 验证标签拖拽和后台排序参数，以及快捷键和关闭后的相邻选择。
func TestSessionTabDragOrder(t *testing.T) {
	a := testApp(t)
	a.st.Projects = append(a.st.Projects, proto.Project{ID: "p2", Name: "other project"})
	a.st.Sessions[0].Name = "first tab"
	base := a.st.Sessions[0]
	for _, item := range []struct{ id, project, name string }{
		{"other", "p2", "other tab"}, {"s2", "p1", "second tab"}, {"s3", "p1", "third tab"},
	} {
		s := base
		s.ID, s.Project, s.Name = item.id, item.project, item.name
		a.st.Sessions = append(a.st.Sessions, s)
	}
	moves := make(chan proto.MoveParams, 8)
	cl, stop := projectClient(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "move" {
			var p proto.MoveParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			moves <- p
		}
		return true, nil
	})
	defer stop()
	a.local().cl, a.local().conn = cl, connReady
	a.terms["a"], a.terms["b"] = &termEntry{}, &termEntry{}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	checkOrder := func(want ...string) {
		t.Helper()
		if got := a.visibleOrder(); !slices.Equal(got, want) || a.selected != "s1" {
			t.Fatalf("标签顺序 %q，期望 %q；当前会话 %q", got, want, a.selected)
		}
		var right float32
		for _, id := range want {
			r, ok := tt.Find(a.session(id).DisplayName(a.set.SessionName))
			if !ok || r.X < right {
				t.Fatalf("标签 %q 的显示位置不符合顺序：%v", id, r)
			}
			right = r.X + r.W
		}
	}
	drag := func(source, target string, after bool) {
		t.Helper()
		from, ok := tt.Find(source)
		to, ok2 := tt.Find(target)
		if !ok || !ok2 {
			t.Fatalf("找不到拖拽标签 %q 或目标 %q", source, target)
		}
		x := to.X + 3
		if after {
			x = to.X + to.W - 3
		}
		tt.Press(from.X+40, from.Y+from.H/2)
		tt.Move(x, to.Y+to.H/2)
		if source != target {
			shot(t, tt, "session-tab-drag-preview")
		}
		tt.Release(x, to.Y+to.H/2)
		tt.Frame()
	}
	checkMove := func(id string, index int) {
		t.Helper()
		select {
		case p := <-moves:
			if p.ID != id || p.Index != index || p.Project != "" {
				t.Fatalf("后台排序参数错误：%+v", p)
			}
		case <-time.After(time.Second):
			t.Fatal("未收到后台排序请求")
		}
	}
	drag("third tab", "first tab", false)
	checkOrder("s3", "s1", "s2")
	checkMove("s3", 0)
	drag("third tab", "second tab", true)
	checkOrder("s1", "s2", "s3")
	checkMove("s3", 2)
	drag("second tab", "first tab", false)
	checkOrder("s2", "s1", "s3")
	checkMove("s2", 0)
	drag("second tab", "third tab", true)
	checkOrder("s1", "s3", "s2")
	checkMove("s2", 2)
	drag("second tab", "second tab", false)
	checkOrder("s1", "s3", "s2")
	from, _ := tt.Find("second tab")
	to, _ := tt.Find("first tab")
	tt.Press(from.X+40, from.Y+from.H/2)
	tt.Move(to.X+3, to.Y+to.H/2)
	checkOrder("s1", "s3", "s2")
	tt.Key(0, ui.KeyEscape)
	checkOrder("s1", "s3", "s2")
	tt.Release(to.X+3, to.Y+to.H/2)
	tt.Frame()
	checkOrder("s1", "s3", "s2")
	a.moveSessionTab("s2", "other", false)
	a.moveSessionTab("missing", "s1", false)
	select {
	case p := <-moves:
		t.Fatalf("取消、原位或跨项目操作发送了排序请求：%+v", p)
	default:
	}
	for _, project := range []string{"other project", "demo"} {
		if err := tt.Click(project); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	checkOrder("s1", "s3", "s2")
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if a.selected != "s3" {
		t.Fatalf("数字快捷键未跟随新顺序：%q", a.selected)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if a.selected != "s2" {
		t.Fatalf("下一标签快捷键未跟随新顺序：%q", a.selected)
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	tt.Frame()
	if a.selected != "s3" {
		t.Fatalf("上一标签快捷键未跟随新顺序：%q", a.selected)
	}
	a.closeSession("s3", false)
	tt.Frame()
	if a.selected != "s2" || !slices.Equal(a.visibleOrder(), []string{"s1", "s2"}) {
		t.Fatalf("关闭标签后顺序或选择错误：%q，%q", a.visibleOrder(), a.selected)
	}
	shot(t, tt, "session-tab-drag-order")
	tt.Advance(time.Second)
	for range 2 {
		if err := tt.Click("first tab"); err != nil {
			t.Fatal(err)
		}
	}
	tt.Frame()
	if a.renaming != "s1" {
		t.Fatalf("双击标签未进入重命名：%q", a.renaming)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.renaming != "" {
		t.Fatal("Esc 未取消标签重命名")
	}
}

// TestProjectDragOrder 验证项目拖拽、跨主机排序、取消操作和持久化。
func TestProjectDragOrder(t *testing.T) {
	a := testApp(t)
	a.st.Projects = append(a.st.Projects,
		proto.Project{ID: "p2", Name: "other project", Dir: "/tmp/other"},
		proto.Project{ID: "p3", Name: "last project", Dir: "/tmp/last"})
	a.mergeState(a.local(), a.st)
	withHost(t, a, "build")
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	checkOrder := func(want ...string) {
		t.Helper()
		var got []string
		var bottom float32
		for _, g := range a.groups() {
			got = append(got, g.key())
			r, ok := tt.Find(g.project.Name)
			if !ok || r.Y < bottom {
				t.Fatalf("项目 %q 的显示位置不符合顺序：%v", g.key(), r)
			}
			bottom = r.Y + r.H
		}
		if !slices.Equal(got, want) || a.selected != "s1" {
			t.Fatalf("项目顺序 %q，期望 %q；当前会话 %q", got, want, a.selected)
		}
	}
	drag := func(source, target string, after bool) {
		t.Helper()
		from, ok := tt.Find(source)
		to, ok2 := tt.Find(target)
		if !ok || !ok2 {
			t.Fatalf("找不到拖拽项目 %q 或目标 %q", source, target)
		}
		y := to.Y + 2
		if after {
			y = to.Y + to.H - 2
		}
		tt.Press(from.X+24, from.Y+from.H/2)
		tt.Move(to.X+24, y)
		if source != target {
			shot(t, tt, "project-drag-preview")
		}
		tt.Release(to.X+24, y)
		tt.Frame()
	}
	checkOrder("p1", "p2", "p3", "rp-build")
	drag("api", "demo", false)
	checkOrder("rp-build", "p1", "p2", "p3")
	drag("api", "last project", true)
	checkOrder("p1", "p2", "p3", "rp-build")
	drag("other project", "demo", false)
	checkOrder("p2", "p1", "p3", "rp-build")
	drag("other project", "last project", true)
	checkOrder("p1", "p3", "p2", "rp-build")
	drag("other project", "other project", false)
	checkOrder("p1", "p3", "p2", "rp-build")
	from, _ := tt.Find("last project")
	to, _ := tt.Find("demo")
	tt.Press(from.X+24, from.Y+from.H/2)
	tt.Move(to.X+24, to.Y+2)
	tt.Key(0, ui.KeyEscape)
	tt.Release(to.X+24, to.Y+2)
	tt.Frame()
	checkOrder("p1", "p3", "p2", "rp-build")
	tt.Press(from.X+24, from.Y+from.H/2)
	tt.Move(900, 600)
	tt.Release(900, 600)
	tt.Frame()
	checkOrder("p1", "p3", "p2", "rp-build")
	restored := newApp()
	if !slices.Equal(restored.prefs.ProjectOrder, a.prefs.ProjectOrder) {
		t.Fatalf("重启后项目顺序未恢复：%q", restored.prefs.ProjectOrder)
	}
	projects := a.projectsOf(a.local())
	slices.Reverse(projects)
	a.mergeState(a.local(), proto.State{Projects: projects, Sessions: a.sessionsOf(a.local())})
	tt.Frame()
	checkOrder("p1", "p3", "p2", "rp-build")
	a.st.Projects = append(a.st.Projects, proto.Project{ID: "p4", Name: "new project", Dir: "/tmp/new"})
	tt.Frame()
	checkOrder("p1", "p3", "p2", "rp-build", "p4")
	shot(t, tt, "project-drag-order")
	tt.SetDark(true)
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "project-drag-order-dark")
}

// TestProjectOrderWhileHostLoads 验证本地排序不会丢失尚未加载的远程项目位置。
func TestProjectOrderWhileHostLoads(t *testing.T) {
	a := testApp(t)
	a.st.Projects = append(a.st.Projects, proto.Project{ID: "p2", Name: "other project"})
	a.mergeState(a.local(), a.st)
	h := withHost(t, a, "build")
	remote := proto.State{Projects: a.projectsOf(h), Sessions: a.sessionsOf(h)}
	a.prefs.ProjectOrder = []string{"rp-build", "p2", "p1"}
	a.mergeState(h, proto.State{})
	h.loaded, h.conn = false, connConnecting
	a.moveProject("p1", "p2", false)
	if !slices.Equal(a.prefs.ProjectOrder, []string{"rp-build", "p1", "p2"}) {
		t.Fatalf("尚未加载的远程项目顺序丢失：%q", a.prefs.ProjectOrder)
	}
	a.mergeState(h, remote)
	h.loaded, h.conn = true, connReady
	var got []string
	for _, g := range a.groups() {
		got = append(got, g.key())
	}
	if !slices.Equal(got, []string{"rp-build", "p1", "p2"}) {
		t.Fatalf("远程主机加载后项目顺序错误：%q", got)
	}
}

// TestSSHModeIsolation 验证同一项目中的 SSH 会话不会混入终端列表或标签，并分别恢复选择。
func TestSSHModeIsolation(t *testing.T) {
	a := testApp(t)
	regular := a.selected
	ssh := a.st.Sessions[0]
	ssh.ID, ssh.Name = "ssh-isolation", "SSH isolated"
	ssh.Pane = proto.Pane{ID: "ssh-pane", Command: []string{"/app/panenest", "__ssh-terminal", "/config", "server"}}
	ssh.Panes = []proto.Pane{ssh.Pane}
	a.st.Sessions = append(a.st.Sessions, ssh)
	for _, g := range a.groups() {
		for _, s := range g.sessions {
			if sshSessionName(s) != "" {
				t.Fatal("SSH 会话混入普通项目列表")
			}
		}
	}
	a.toggleSSHList()
	if !a.prefs.SSHList || a.selected != ssh.ID {
		t.Fatalf("未恢复 SSH 模式：%q", a.selected)
	}
	g, ok := a.currentGroup()
	if !ok || len(g.sessions) != 1 || g.sessions[0].ID != ssh.ID {
		t.Fatalf("SSH 标签混入本地会话：%+v", g)
	}
	a.toggleSSHList()
	if a.prefs.SSHList || a.selected != regular {
		t.Fatalf("未恢复终端选择：%q", a.selected)
	}
	g, ok = a.currentGroup()
	if !ok {
		t.Fatal("未恢复终端项目")
	}
	for _, s := range g.sessions {
		if sshSessionName(s) != "" {
			t.Fatal("终端标签混入 SSH 会话")
		}
	}
	a.toggleSSHList()
	if a.selected != ssh.ID {
		t.Fatal("再次切换丢失 SSH 选择")
	}
	a.selectSSHConnection(settings.Host{Name: "empty-server"})
	if a.selected != "" || a.prefs.SSHConnection != "empty-server" {
		t.Fatal("空连接残留其他连接的终端")
	}
	g, ok = a.currentGroup()
	if !ok || len(g.sessions) != 0 {
		t.Fatal("空连接混入已有终端标签")
	}
	a.reselect()
	if a.selected != "" {
		t.Fatal("后台状态刷新自动选中了其他连接")
	}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if tt.HasText("SSH isolated") {
		t.Fatal("空连接右侧显示其他 SSH 标签")
	}
	a.toggleSSHList()
	a.toggleSSHList()
	if a.selected != "" || a.prefs.SSHConnection != "empty-server" {
		t.Fatal("模式切换未保留空连接选择")
	}
	a.selectSSHConnection(settings.Host{Name: "server"})
	if a.selected != ssh.ID {
		t.Fatal("连接未恢复自己的终端")
	}

	a.newSession("ssh:server")
	if a.selected != "" || !a.prefs.SSHBlank {
		t.Fatal("SSH 新建标签未打开空白页")
	}
	a.reselect()
	if a.selected != "" {
		t.Fatal("后台刷新覆盖 SSH 空白页")
	}
	tt.Frame()
	if !tt.HasText("新标签页") {
		t.Fatal("未显示 SSH 空白标签")
	}
	if !tt.HasText("点击下方按钮打开 SSH 终端") {
		t.Fatal("空白页未显示连接提示")
	}
	a.selectSession(ssh.ID)
	if a.prefs.SSHBlank {
		t.Fatal("切回已有 SSH 标签后仍保留空白状态")
	}

}

// TestSSHConnectionDragOrder 验证拖拽连接可持久化排序，且不影响选择和其他主机。
func TestSSHConnectionDragOrder(t *testing.T) {
	a := testApp(t)
	a.prefs.SSHList, a.prefs.SSHConnection, a.selected = true, "one", ""
	a.set.Hosts = []settings.Host{{Name: "one", Addr: "root@one", Password: "saved"}, {Name: "remote", Addr: "http://remote:7373"}, {Name: "two", Addr: "root@two"}, {Name: "three", Addr: "root@three"}}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	from, ok := tt.Find("连接 three")
	to, ok2 := tt.Find("连接 one")
	if !ok || !ok2 {
		t.Fatal("找不到 SSH 拖拽条目")
	}
	tt.Press(from.X+24, from.Y+from.H/2)
	tt.Move(to.X+24, to.Y+2)
	tt.Release(to.X+24, to.Y+2)
	tt.Frame()
	var names []string
	for _, h := range a.set.Hosts {
		names = append(names, h.Name)
	}
	if !slices.Equal(names, []string{"three", "remote", "one", "two"}) {
		t.Fatalf("拖拽后的顺序错误：%v", names)
	}
	if a.prefs.SSHConnection != "one" || a.selected != "" {
		t.Fatal("拖拽改变了连接选择或创建了终端")
	}
	saved := settings.Load()
	if !slices.Equal(saved.Hosts, a.set.Hosts) || saved.Host("one").Password != "saved" {
		t.Fatal("排序未正确保存连接信息")
	}
}

// TestNodeVersionBadges 验证明暗主题下项目和标签页的版本徽标，以及清除配置后的移除。
func TestNodeVersionBadges(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := map[bool]string{false: "light", true: "dark"}[dark]
		t.Run(name, func(t *testing.T) {
			a := testApp(t)
			a.prefs.SidebarWidth = minSidebar
			a.st.Projects[0].Name = "A project with a very long name"
			a.st.Projects[0].NodeVersion = "22.14.0"
			a.st.Sessions[0].Name = "A tab with a very long name"
			a.st.Sessions[0].NodeVersion = "20.18.0"
			tt := ui.NewTester(a.view, 1100, 640)
			tt.SetDark(dark)
			tt.Frame()
			tt.Advance(time.Second)
			for _, version := range []string{"v22.14.0", "v20.18.0"} {
				if !tt.HasText(version) {
					t.Fatalf("missing badge %s: %q", version, tt.Texts())
				}
			}
			tab, ok := tt.Find(a.st.Sessions[0].Name)
			close, found := tt.Find(locales.ActionCloseSession())
			if !ok || !found || close.W < 20 || close.X+close.W > tab.X+tab.W {
				t.Fatalf("close button %v outside tab %v", close, tab)
			}
			shot(t, tt, "node-version-badges-"+name)
			a.st.Projects[0].NodeVersion = ""
			a.st.Sessions[0].NodeVersion = ""
			tt.Frame()
			if tt.HasText("v22.14.0") || tt.HasText("v20.18.0") {
				t.Fatal("cleared version badge remains")
			}
		})
	}
}
