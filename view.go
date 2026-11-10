package main

import (
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"panenest/internal/proto"
	"panenest/internal/settings"

	"panenest/locales"
)

const (
	cardGap = 8 // around the card
	// tabBarH 和 rowH 独立控制会话标签栏与标签高度。
	tabBarH = 32
	rowH    = 32
	// barMid 统一窗口按钮、侧栏操作按钮和标签栏的纵向中心。
	barMid = cardGap + tabBarH/2
	topBar = 2 * barMid
	// headerH 独立保留窗格标题栏的高度。
	headerH = 44
	// rowInset 是项目列表和会话标签的文字左侧内边距。
	rowInset = 12
)

// chrome is where the sidebar's animation puts things, in whole pixels so
// that edges stay crisp. Each is rounded once, from one progress quantized
// to the sidebar's pixels, so that they move together: rounded apart,
// they would step at different frames, and a title, their sum, would
// shake by a pixel as the spring settles.
type chrome struct {
	shown     float32 // the sidebar's progress, 0 hidden to 1 shown
	sideW     float32 // the sidebar's width as shown
	mainPad   float32 // the left padding of the cards' column
	headerPad float32 // the left padding of the header at the left edge
}

// lead is where the sidebar's buttons start in the window, with the sidebar
// shown or not: after the traffic lights on macOS; elsewhere inside the
// card's edge, which they come over as the sidebar hides.
func lead(bar ui.TitleBar) float32 {
	if ownControls {
		return bar.Left + 2*cardGap
	}
	return bar.Left + cardGap
}

var sidebarMotion = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.CubicBezier(0.25, 1, 0.5, 1)}

func chromeFor(progress, width float32, bar ui.TitleBar) chrome {
	round := func(v float32) float32 { return float32(math.Round(float64(v))) }
	w := round(width * min(max(progress, 0), 1))
	s := w / width
	pad := round(cardGap * (1 - s))
	// Where the header's title starts in the window, rounded once: after
	// the traffic lights and the sidebar's buttons with the sidebar
	// hidden, 16 into the card with it shown.
	hidden, shown := lead(bar)+16+88, width+16
	x := round(hidden + (shown-hidden)*s)
	return chrome{shown: s, sideW: w, mainPad: pad, headerPad: x - w - pad}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (a *App) view(c *ui.Context) {
	a.looks()
	pal := paletteFor(c)
	c.SetTheme(borderless(c, pal))

	if a.pendingToast != "" {
		c.Toast(a.pendingToast)
		a.pendingToast = ""
	}
	a.shortcuts(c)

	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	backdrop(c, row)
	// A curve, not a spring: a spring's long tail crawls under a pixel a
	// frame, which whole pixels turn into hops and holds, a shake as it
	// settles. This one lands in about 13 frames, moving in every one.
	progress := row.AnimateTo("sidebar", b2f(!a.prefs.SidebarHidden), sidebarMotion)
	a.chrome = chromeFor(progress, a.prefs.SidebarWidth, bar)
	shown := a.chrome.shown
	sideShown := row.AnimateTo("side-panel", b2f(a.prefs.Side != ""), sidebarMotion)
	if a.prefs.Side != "" {
		a.side.visible = a.prefs.Side
	} else if sideShown == 0 {
		a.side.visible = ""
	}
	row.Children(func() {
		w := a.chrome.sideW
		side := ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).ClipX()
		if h := a.local(); h.conn == connReady && h.cl != nil && !a.prefs.SidebarHidden && !a.pal.open && !a.picker.open && (a.confirm == nil || !a.confirm.open) {
			if files := side.DroppedFiles(); len(files) > 0 {
				go a.openProjectDirs(h, h.cl, slices.Clone(files), true)
			}
			if side.FileDragOver() {
				side.Background(pal.accent.Alpha(0.08)).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Stroke(ui.Rect{X: r.X + 4, Y: r.Y + 4, W: r.W - 8, H: r.H - 8}, pal.accent.Alpha(0.55), 8, 1)
				})
			}
		}
		side.Children(func() {
			ui.Column(c).Width(a.prefs.SidebarWidth).Shrink(0).Margin(0, 0, 0, w-a.prefs.SidebarWidth).
				Opacity(0.4 + 0.6*shown).Children(func() { a.sidebar(c, pal, bar) })
		})
		// The gaps around the cards move the window too.
		main := ui.Column(c).Grow(1).MinWidth(0).Padding(cardGap, 4, cardGap, a.chrome.mainPad).DragWindow()
		main.Children(func() {
			if shown > 0.01 {
				a.resizeHandle(c, pal)
			}
			a.card(c, pal, bar, shown, sideShown)
		})
		a.rightRail(c, pal)
	})
	// A page staged for a screenshot takes the window's size, under its
	// content, which hides it: at no opacity, it paints no hole there.
	for id, e := range a.webs {
		if e.staged && e.view != nil {
			ui.Box(c.Key("staged-" + id)).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Stretch).Opacity(0).Children(func() {
				ui.WebView(c.Key("web"), e.view).Grow(1)
			})
		}
	}
	a.sidebarButtons(c, pal, bar)
	a.paletteView(c, pal)
	a.pickerView(c, pal)
	a.confirmView(c, pal)
	a.worktreeView(c, pal)
	a.nodeVersionDialog(c, pal)
	a.snippetDialog(c, pal)
	a.sshDialog(c, pal)
	// A preview that showed no more fades in anew next time.
	if !a.previewShown {
		a.previewKey = ""
	}
	a.previewShown = false
	// An image previewed no more closes, and lets go of its bitmap.
	if !a.peek.shown {
		a.closePeek()
		a.peek = peek{}
	}
	a.peek.shown = false
	if later := a.later; len(later) > 0 {
		a.later = nil
		for _, fn := range later {
			fn()
		}
		c.Invalidate()
	}
}

// resizeHandle is the sidebar's edge, which a drag moves.
func (a *App) resizeHandle(c *ui.Context, pal *palette) {
	// A drag leaves the keyboard in the pane the user is in.
	h := ui.Box(c.Key("resize")).Absolute().Left(-4).Top(0).Bottom(0).Width(9).Cursor(ui.CursorResizeColumn).KeepFocus()
	if dx, _, ok := h.Dragged(); ok {
		if a.sideDrag == 0 {
			a.sideDrag = a.prefs.SidebarWidth
		}
		a.sideDrag += dx
		a.prefs.SidebarWidth = min(max(a.sideDrag, minSidebar), maxSidebar)
	} else if a.sideDrag != 0 && !h.Pressed() {
		a.sideDrag = 0
		a.savePrefs()
	}
	if h.DoubleClicked() {
		a.prefs.SidebarWidth = defaultSidebar
		a.savePrefs()
	}
}

// iconButton 创建工具栏图标按钮，并返回按钮以便调整布局。
func iconButton(c *ui.Context, key string, svg *ui.SVG, label string, pal *palette, fn func()) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(label)
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover)
		b.TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(16, 16) })
	if b.Clicked() {
		fn()
	}
	return b
}

// tipsAt are where pointerTooltip's tips show, by key: where the pointer
// rested as each came, which they keep while they show.
var tipsAt = map[string]ui.Point{}

// pointerTooltip shows s as el's tooltip, as Element.Tooltip does, but
// where macOS shows its tooltips: below the pointer where it came to rest,
// clear of the arrow, rather than centered above el, which a long row
// puts far from where the user looks.
func pointerTooltip(c *ui.Context, el ui.Element, key, s string) {
	el.Description(s)
	const below = 18 // the arrow's height, from its tip
	px, py, over := el.PointerPosition()
	at, showing := tipsAt[key]
	if !showing {
		b := el.Bounds()
		at = ui.Point{X: b.X + px, Y: b.Y + py + below}
	}
	var spot ui.Element
	if over {
		ui.Overlay(c, func() {
			spot = ui.Box(c.Key(key+"-at")).Absolute().Left(at.X).Top(at.Y).Size(0, 0).PassThrough()
		})
	}
	shown := false
	ui.TooltipBase(c, el, func(tip ui.Element) {
		shown = true
		th := c.Theme()
		fill, text := th.Inverse, th.InverseText
		if fill == ui.Transparent {
			fill = th.Text
		}
		if text == ui.Transparent {
			text = th.Background
		}
		tip.MaxWidth(th.Space(80)).Padding(th.Space(1.25), th.Space(2)).Radius(th.Space(1.25)).
			Background(fill).TextColor(text).FontSize(th.FontSize-1).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.2))
		tip.Children(func() { ui.Text(c, s) })
		if over {
			tip.AttachTo(spot, ui.AnchorTopLeft, ui.AnchorTopLeft)
		}
	})
	if shown {
		tipsAt[key] = at
	} else {
		delete(tipsAt, key)
	}
}

func kbd(c *ui.Context, pal *palette, keys string) {
	ui.Text(c, keys).FontSize(11).TextColor(pal.textMuted).Padding(1, 5).Radius(4).
		Background(pal.kbd).SingleLine()
}

// --- the sidebar ---

func (a *App) sidebar(c *ui.Context, pal *palette, bar ui.TitleBar) {
	ui.Row(c).Height(topBar).Shrink(0).DragWindow()

	if a.prefs.SSHList {
		a.sshSidebar(c, pal)
		a.sidebarFooter(c, pal)
		return
	}
	// The space between and below the rows moves the window, as the top
	// bar does.
	ui.Scroll(c.Key("projects")).Grow(1).Padding(8, 10, 16, 10).DragWindow().Children(func() {
		gi := 0 // the groups shown, the first of which sits higher
		seen := map[*host]bool{}
		showHost := func(h *host) {
			if h.remote() && a.hostNeedsRow(h) {
				top := float32(18)
				if gi == 0 {
					top = 2
				}
				a.hostLine(c, pal, h, top)
				gi++
			}
		}
		// 项目按用户保存的顺序平铺；主机状态跟随其第一个项目。
		for _, g := range a.groups() {
			if !g.host.remote() && !g.host.loaded {
				continue
			}
			g.first = !seen[g.host]
			if g.first {
				showHost(g.host)
				seen[g.host] = true
			}
			a.groupView(c, pal, g, gi)
			gi++
		}
		for _, h := range a.hosts[1:] {
			if !seen[h] {
				showHost(h)
			}
		}
		if local := a.local(); local.loaded && len(a.st.Sessions) == 0 && local.conn == connReady && len(a.hosts) == 1 {
			ui.Text(c, locales.SidebarNoSessions()).FontSize(12.5).TextColor(pal.textFaint).Padding(8, 10)
		}
		if a.set.SessionHistory {
			a.endedSections(c, pal)
		}
	})
	a.sidebarFooter(c, pal)
	c.After(20 * time.Second) // times ago move on
}

// projectDrag 用项目标识区分侧栏排序与窗格拖拽，兼容未分组会话。
type projectDrag struct{ key string }

// groupView 绘制用于切换项目的平级列表项，保留项目管理菜单和会话状态汇总。
func (a *App) groupView(c *ui.Context, pal *palette, g group, gi int) {
	id, key := g.project.ID, g.key()
	selected := a.prefs.Project == key
	if s := a.session(a.selected); s != nil {
		selected = a.sessionProjectKey(s) == key
	}
	needs := 0
	for _, s := range g.sessions {
		if s.Status == proto.Waiting {
			needs++
		}
	}
	top := float32(4)
	if gi == 0 {
		top = 2
	}
	hdr := ui.ButtonBase(c.Key("project-"+key)).Height(38).Margin(top, 0, 0, 0).Padding(0, 8, 0, rowInset).Gap(6).AlignItems(ui.Center).Radius(8).
		Label(g.project.Name).Selected(selected).TextColor(pal.text)
	hovered := hdr.Hovered()
	if pal.dark {
		hdr.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	if selected {
		bg = pal.rowSelected
		chosen(hdr, pal)
	} else if hovered {
		bg = pal.rowHover
	}
	hdr.Background(hdr.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if g.host.remote() && g.host.conn != connReady {
		hdr.Opacity(0.55)
	}
	renaming := id != "" && a.renamingProject == id
	if !renaming {
		hdr.Drag(projectDrag{key})
		if hdr.Dragging() {
			hdr.Opacity(0.45)
		}
		_, y, _ := hdr.PointerPosition()
		after := y >= hdr.Bounds().H/2
		if d, over := ui.DragOver[projectDrag](hdr); over && d.key != key {
			hdr.DrawOver(func(p *ui.Painter, r ui.Rect) {
				if after {
					r.Y += r.H - 2
				}
				p.Fill(ui.Rect{X: r.X + 4, Y: r.Y, W: r.W - 8, H: 2}, pal.accent, 1)
			})
		}
		if d, ok := ui.Drop[projectDrag](hdr); ok && d.key != key {
			a.defer_(func() { a.moveProject(d.key, key, after) })
		}
	}
	if hdr.Clicked() && !renaming {
		a.defer_(func() { a.selectProject(key) })
	}
	pointerTooltip(c, hdr, "tip-project-"+key, g.host.shortDir(g.project.Dir))
	hdr.ContextMenu(func(m *ui.Menu) {
		if m.Item("设置 Node 版本…").Disabled(id == "").Chosen() {
			a.nodeOpen, a.nodeTarget, a.nodeMethod, a.nodeDraft = true, id, "project.node-version", g.project.NodeVersion
		}
		if m.Item(locales.ActionNewSession()).Chosen() {
			a.newSessionOn(g.host, id)
		}
		if m.Item(locales.SessionRename()).Disabled(id == "").Chosen() {
			a.startProjectRename(id)
		}
		m.Separator()
		// A remote group's menu has its host's, as the host has no header.
		if g.host.remote() {
			m.Submenu(g.host.name, func(m *ui.Menu) { a.hostMenu(m, g.host) })
		}
		// Close All Sessions ends the sessions the project holds, not the
		// project itself, asking first as closing the project does: what
		// runs in them ends with them.
		if m.Item(locales.SidebarCloseAllSessions()).Disabled(len(g.sessions) == 0).Chosen() {
			name, count := g.project.Name, len(g.sessions)
			a.confirm = &confirmation{
				open:    true,
				title:   locales.ConfirmCloseAllSessionsTitle(name),
				detail:  locales.ConfirmCloseAllSessionsDetail(count),
				action:  locales.SidebarCloseAllSessions(),
				destroy: true,
				run:     func() { a.closeAllSessions(g) },
			}
		}
		if m.Item(locales.SidebarCloseProjectMenu()).Disabled(id == "").Chosen() {
			name, count := g.project.Name, len(g.sessions)
			a.confirm = &confirmation{
				open:    true,
				title:   locales.ConfirmCloseTitle(name),
				detail:  locales.ConfirmCloseProjectDetail(count),
				action:  locales.SidebarCloseProject(),
				destroy: true,
				run:     func() { a.call(id, "project.close", proto.IDParams{ID: id}, nil) },
			}
		}
	})
	hdr.Children(func() {
		if renaming {
			in := ui.TextInputBase(c.Key("rename"), &a.renameBuf).Grow(1).MinWidth(0).FontSize(12.5).FontWeight(400).AutoFocus()
			if in.Shortcut(0, ui.KeyEscape) {
				a.renamingProject = ""
				a.focusTerminal()
			} else if in.Submitted() || (!in.Focused() && !in.Pressed() && in.Valid() && a.renameStarted(c)) {
				a.commitProjectRename()
			}
			return
		}
		fg := pal.textMuted
		if selected {
			fg = pal.text
		}
		ui.Text(c, g.project.Name).FontSize(13.5).FontWeight(400).TextColor(fg).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		nodeVersionBadge(c, pal, g.project.NodeVersion)
		if g.host.remote() {
			a.hostTag(c, pal, g.host)
		}
		ui.Textf(c, "%d", len(g.sessions)).FontSize(12.5).FontWeight(400).TextColor(pal.textFaint.Alpha(0.75)).FontFeatures("tnum")
		ui.Spacer(c)
		// A host with no row of its own tells how the app stands with it
		// here: reconnecting, or wanting an update or restart.
		if g.first && g.host.remote() && !a.hostNeedsRow(g.host) {
			a.hostState(c, pal, g.host)
		}
		if needs > 0 {
			ui.Text(c, locales.SidebarNeedsYou(needs)).FontSize(12.5).FontWeight(500).TextColor(pal.attention).SingleLine()
		}
		if hovered {
			b := ui.ButtonBase(c.Key("add")).Size(20, 20).Radius(5).Center().TextColor(pal.textMuted).Label(locales.NewSessionIn(g.project.Name)).Tooltip(locales.ActionNewSession())
			if b.Hovered() {
				b.Background(pal.rowHover).TextColor(pal.text)
			}
			b.Children(func() { ui.Icon(c, plusIcon).Size(13, 13) })
			if b.Clicked() {
				a.newSessionOn(g.host, id)
			}
		}
	})
}

// sessionTabs 在主区域顶部显示当前项目的会话，溢出时横向滚动。
func (a *App) sessionTabs(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32, g group) {
	blank := a.prefs.SSHList && a.prefs.SSHBlank
	ids := make([]string, len(g.sessions))
	for i, s := range g.sessions {
		ids[i] = s.ID
	}
	if blank {
		ids = append(ids, "ssh-blank:"+a.prefs.SSHConnection)
	}
	if a.tabSession != a.selected || !slices.Equal(a.tabOrder, ids) {
		a.tabIndex = 0
		for i, s := range g.sessions {
			if s.ID == a.selected {
				a.tabIndex = i
			}
		}
		if blank {
			a.tabIndex = len(g.sessions)
		}
		a.tabSession = a.selected
		a.tabOrder = ids
		a.revealTab = a.selected
	}
	ui.Row(c.Key("session-tabs")).Height(tabBarH).Shrink(0).MinWidth(0).AlignItems(ui.Center).
		Padding(0, 8, 0, a.chrome.headerPad).Gap(6).DragWindow().Children(func() {
		ui.ScrollHorizontal(c.Key("tab-scroll-" + g.key())).MinWidth(0).Height(tabBarH).Children(func() {
			tabs := ui.TabsBase(c.Key("tabs"), &a.tabIndex, len(ids))
			tabs.List.Height(tabBarH).AlignItems(ui.Center).Gap(6).Children(func() {
				for i, s := range g.sessions {
					ui.Box(c.Key(s.ID)).Shrink(0).Children(func() {
						a.sessionTab(c, pal, s, tabs.Tab(i))
					})
				}
				if blank {
					tabs.Tab(len(g.sessions)).Size(170, rowH).Padding(0, 8, 0, rowInset).Radius(8).Background(pal.rowSelected).Label("新标签页").AlignItems(ui.Center).Children(func() {
						ui.Text(c, "新标签页").FontSize(13).TextColor(pal.text)
						ui.Spacer(c)
						iconButton(c, "close-blank", closeIcon, "关闭空白页", pal, func() { a.defer_(func() { a.selectSSHConnection(settings.Host{Name: a.prefs.SSHConnection}) }) })
					})
				}
			})
			if tabs.List.Changed() && a.tabIndex < len(g.sessions) {
				id := g.sessions[a.tabIndex].ID
				a.defer_(func() { a.selectSession(id) })
			}
		})
		iconButton(c, "add-tab", plusIcon, a.tip(locales.NewSessionIn(g.project.Name), "session.new"), pal, func() {
			a.defer_(func() { a.newSession(g.project.ID) })
		}).Shrink(0).Margin(0, ui.Auto, 0, 0)
		windowControls(c, pal, a.win)
	})
}

// sessionTabDrag 仅在同一项目的会话标签间传递拖拽标识。
type sessionTabDrag struct{ id string }

// sessionTab 绘制可拖拽排序的会话标签，保留关闭、重命名和详情提示。
func (a *App) sessionTab(c *ui.Context, pal *palette, s *proto.Session, row ui.Element) {
	selected := s.ID == a.selected
	id := s.ID
	row.Size(170, rowH).Padding(0, 8, 0, rowInset).Gap(8).AlignItems(ui.Center).Radius(8).Label(s.DisplayName(a.set.SessionName)).Bind(a.paneRef("tab-" + id))
	row.HandleInput(func(ev ui.InputEvent) bool { return a.sessionTabInput(c, id, ev) })
	if selected && a.revealTab == s.ID {
		row.ScrollIntoView()
		a.revealTab = ""
	}
	hovered := row.Hovered()
	if pal.dark {
		// Room for the chosen row's edge in every row, so that choosing
		// one moves no text.
		row.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.rowSelected
		chosen(row, pal)
	case hovered:
		bg = pal.rowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	// A session of a host the app lost shows as it was, faded, until the
	// host is back.
	if h := a.hostOf(s.ID); h.remote() && h.conn != connReady {
		row.Opacity(0.55)
	}
	if a.renaming != id {
		row.Drag(sessionTabDrag{id})
		if row.Dragging() {
			row.Opacity(0.45)
		}
		x, _, _ := row.PointerPosition()
		after := x >= a.paneRef("tab-"+id).Bounds(c).W/2
		if d, over := ui.DragOver[sessionTabDrag](row); over && d.id != id {
			row.DrawOver(func(p *ui.Painter, r ui.Rect) {
				if after {
					r.X += r.W - 2
				}
				p.Fill(ui.Rect{X: r.X, Y: r.Y + 4, W: 2, H: r.H - 8}, pal.accent, 1)
			})
		}
		if d, ok := ui.Drop[sessionTabDrag](row); ok && d.id != id {
			a.defer_(func() { a.moveSessionTab(d.id, id, after) })
		}
	}
	tip := s.WorkDir()
	if h := a.hostOf(s.ID); h.remote() {
		tip = h.name + ":" + tip
	}
	if s.Process != "" {
		tip = s.Process + " · " + tip
	}
	if s.Message != "" {
		tip = s.Message + "\n" + tip
	}
	g := sessionGit(s)
	if g != nil {
		tip += "\n" + gitSummary(g)
	}
	tip = s.DisplayName(a.set.SessionName) + "\n" + tip
	pointerTooltip(c, row, "tip-"+s.ID, tip)
	row.ContextMenu(func(m *ui.Menu) { a.sessionMenu(m, s) })
	row.Children(func() {
		if a.renaming == s.ID {
			in := ui.TextInputBase(c.Key("rename"), &a.renameBuf).Grow(1).MinWidth(0).FontSize(13.5).AutoFocus()
			if in.Submitted() || in.Shortcut(0, ui.KeyEscape) {
				if in.Shortcut(0, ui.KeyEscape) {
					a.renaming = ""
					a.focusTerminal()
				} else {
					a.commitRename()
				}
			} else if !in.Focused() && !in.Pressed() && in.Valid() && a.renameStarted(c) {
				a.commitRename()
			}
			return
		}
		name := ui.Text(c, s.DisplayName(a.set.SessionName)).FontSize(13.5).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).TextColor(pal.text)
		if s.Unread {
			name.FontWeight(650)
		}
		if s.Status == proto.Exited && !s.Unread {
			name.TextColor(pal.textMuted)
		}
		nodeVersionBadge(c, pal, s.NodeVersion)
		close := ui.ButtonBase(c.Key("close-tab")).Size(20, 20).Shrink(0).Radius(5).Center().TextColor(pal.textFaint).
			Label(locales.ActionCloseSession()).Tooltip(a.tip(locales.ActionCloseSession(), "session.close"))
		if !selected && !hovered {
			close.Invisible()
		}
		if close.Hovered() {
			close.Background(pal.rowHover).TextColor(pal.text)
		}
		close.Children(func() { ui.Icon(c, closeIcon).Size(11, 11) })
		if close.Clicked() {
			a.defer_(func() { a.closeSession(id, true) })
		}
	})
}

// sessionTabInput 在松开时处理双击重命名和中键关闭，让连续拖拽优先于重命名。
func (a *App) sessionTabInput(c *ui.Context, id string, ev ui.InputEvent) bool {
	if ev.Button == 0 && ev.Kind == ui.InputPointerUp && ev.Clicks == 2 {
		bounds := a.paneRef("tab-" + id).Bounds(c)
		if ev.X >= 0 && ev.Y >= 0 && ev.X < bounds.W && ev.Y < bounds.H {
			a.defer_(func() { a.startRename(id) })
		}
		return false
	}
	if ev.Button != 2 {
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		a.tabClose = id
		return true
	case ui.InputPointerUp:
		pressed := a.tabClose == id
		a.tabClose = ""
		bounds := a.paneRef("tab-" + id).Bounds(c)
		if pressed && ev.X >= 0 && ev.Y >= 0 && ev.X < bounds.W && ev.Y < bounds.H {
			a.defer_(func() { a.closeSession(id, true) })
		}
		return pressed
	}
	return false
}

// renameStarted keeps a rename from ending in the frame it began in,
// before its input took the focus.
func (a *App) renameStarted(c *ui.Context) bool {
	if a.renameFrames < 3 {
		a.renameFrames++
		c.Invalidate()
		return false
	}
	return true
}

func (a *App) sessionMenu(m *ui.Menu, s *proto.Session) {
	id := s.ID
	if m.Item("设置 Node 版本…").Chosen() {
		a.nodeOpen, a.nodeTarget, a.nodeMethod, a.nodeDraft = true, id, "session.node-version", s.NodeVersion
	}
	if a.menuShortcut(m.Item(locales.SessionRename()), "session.rename").Chosen() {
		a.startRename(id)
	}
	if s.Status == proto.Exited {
		if m.Item(locales.CommonRestart()).Chosen() {
			a.restartSession(id)
		}
	}
	if s.Unread {
		if m.Item(locales.SessionMarkRead()).Chosen() {
			a.call(id, "view", proto.ViewParams{ID: id}, func(error) { a.hostOf(id).viewSent = nil; a.reportView() })
		}
	}
	h := a.hostOf(id)
	m.Submenu(locales.SessionMoveToProject(), func(m *ui.Menu) {
		// A session moves among its host's projects alone.
		for _, p := range a.projectsOf(h) {
			if m.Item(p.Name).Checked(p.ID == s.Project).Disabled(p.ID == s.Project).Chosen() {
				pid := p.ID
				a.defer_(func() { a.moveSession(id, pid) })
			}
		}
	})
	m.Separator()
	if a.menuShortcut(m.Item(locales.GitShowChanges()), "changes.toggle").Chosen() {
		a.defer_(func() {
			a.selectSession(id)
			if a.prefs.Side != sideChanges {
				a.toggleSide(sideChanges)
			}
		})
	}
	m.Separator()
	if m.Item(locales.SessionCopyId()).Chosen() {
		a.copyText = id
	}
	if m.Item(locales.SessionCopyDirectory()).Disabled(s.WorkDir() == "").Chosen() {
		a.copyText = s.WorkDir()
	}
	if m.Item(locales.SessionRevealIn(fileManager())).Disabled(s.WorkDir() == "" || h.remote()).Chosen() {
		a.openLink(id, s.WorkDir())
	}
	m.Separator()
	if a.menuShortcut(m.Item(locales.ActionCloseSession()), "session.close").Chosen() {
		a.defer_(func() { a.closeSession(id, true) })
	}
}

// sidebarFooter 在左下角显示会话搜索、设置、明暗切换，以及会话状态汇总或服务连接状态。
func (a *App) sidebarFooter(c *ui.Context, pal *palette) {
	ui.Row(c).Height(36).Shrink(0).Padding(0, 12, 4, 12).Gap(2).AlignItems(ui.Center).DragWindow().Children(func() {
		iconButton(c, "palette", searchIcon, a.tip(locales.ActionGoToSession(), "palette.open"), pal, a.openPalette)
		iconButton(c, "settings", gearIcon, a.tip(locales.SettingsTitle(), "settings.open"), pal, func() { a.openSettings("") })
		appearance, appearanceIcon, appearanceLabel := "dark", moonIcon, locales.SetThemeDark()
		if c.Theme().Dark {
			appearance, appearanceIcon, appearanceLabel = "light", sunIcon, locales.SetThemeLight()
		}
		iconButton(c, "toggle-appearance", appearanceIcon, locales.SetAppearance()+": "+appearanceLabel, pal, func() {
			s := a.set.Clone()
			s.Appearance = appearance
			a.applySettings(s)
		})
		ui.Spacer(c)
		switch a.local().conn {
		case connReady:
			if a.daemonStale() {
				a.restartChip(c, pal)
			}
			waiting := func(s *proto.Session) bool { return s.Status == proto.Waiting }
			working := func(s *proto.Session) bool { return s.Status == proto.Working }
			done := func(s *proto.Session) bool { return s.Status == proto.Idle && s.Unread }
			a.footerChip(c, pal, proto.Waiting, false, waiting, pal.attention, locales.FooterNeedsYou, a.tip(locales.FooterGoToIt(), "session.attention"), a.jumpToAttention)
			a.footerChip(c, pal, proto.Working, false, working, pal.textMuted, locales.FooterWorking, locales.FooterGoToIt(), nil)
			a.footerChip(c, pal, proto.Idle, true, done, pal.textMuted, locales.FooterDone, locales.FooterGoToIt(), nil)
		case connConnecting:
			a.footerNote(c, pal, pal.textFaint, locales.StatusConnecting())
		default:
			a.footerNote(c, pal, pal.danger, locales.StatusDisconnected())
		}
	})
}

// footerChip counts the sessions that are, by is, as their rows' status
// icon and the count, in fg; a click goes to the next of them, by jump,
// or else after the one shown. one and many word its label and tooltip,
// as "%d session needs you". It shows nothing while none is.
func (a *App) footerChip(c *ui.Context, pal *palette, st proto.Status, unread bool, is func(*proto.Session) bool, fg ui.Color, say func(int) string, hint string, jump func()) {
	n := 0
	for i := range a.st.Sessions {
		if (sshSessionName(&a.st.Sessions[i]) != "") != a.prefs.SSHList {
			continue
		}
		// A host the app lost says nothing new, as for the Dock's badge.
		if h := a.hostOf(a.st.Sessions[i].ID); h.remote() && h.conn != connReady {
			continue
		}
		if is(&a.st.Sessions[i]) {
			n++
		}
	}
	if n == 0 {
		return
	}
	words := say(n)
	b := ui.ButtonBase(c.Key("chip-"+string(st))).Height(24).Padding(0, 8, 0, 6).Gap(5).Radius(12).AlignItems(ui.Center).
		Label(words).Tooltip(words + " · " + hint)
	if b.Hovered() {
		b.Background(pal.rowHover)
	}
	b.Children(func() {
		statusIcon(c, pal, st, unread, 12)
		ui.Textf(c, "%d", n).FontSize(12).FontWeight(500).TextColor(fg).FontFeatures("tnum")
	})
	if b.Clicked() {
		if jump == nil {
			jump = func() { a.jumpTo(is) }
		}
		jump()
	}
}

// restartChip offers to restart panenestd into this version of PaneNest, while it
// runs another.
func (a *App) restartChip(c *ui.Context, pal *palette) {
	b := ui.ButtonBase(c.Key("chip-restart")).Height(24).Padding(0, 8, 0, 8).Gap(6).Radius(12).AlignItems(ui.Center).
		Label(locales.CommonRestartService()).Tooltip(locales.UpdateRestartTip())
	if b.Hovered() {
		b.Background(pal.rowHover)
	}
	b.Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(pal.attention)
		ui.Text(c, locales.CommonRestartService()).FontSize(12).FontWeight(500).TextColor(pal.attention).SingleLine()
	})
	if b.Clicked() {
		a.offerRestart()
	}
}

// footerNote says how the connection to panenestd is, by a dot.
func (a *App) footerNote(c *ui.Context, pal *palette, dot ui.Color, label string) {
	ui.Row(c).Gap(7).Padding(0, 6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, label).FontSize(12).TextColor(pal.textFaint).SingleLine().
			Tooltip(locales.FooterServiceTip())
	})
}

// ago is how long ago a time in Unix milliseconds was, shortly, as
// proto.Ago writes it, in the language the app shows.
func ago(ms int64, now time.Time) string {
	if ms == 0 {
		return ""
	}
	d := now.Sub(time.UnixMilli(ms))
	switch {
	case d < time.Minute:
		return locales.AgoNow()
	case d < time.Hour:
		return locales.AgoMinutes(int(d / time.Minute))
	case d < 24*time.Hour:
		return locales.AgoHours(int(d / time.Hour))
	case d < 7*24*time.Hour:
		return locales.AgoDays(int(d / (24 * time.Hour)))
	}
	return locales.AgoWeeks(int(d / (7 * 24 * time.Hour)))
}

// statusIcon draws a session's status, as the sidebar shows it: a ring
// turning while it works, an amber ring when it needs the user, a filled
// check when it finished unseen, a faint check once seen.
func statusIcon(c *ui.Context, pal *palette, st proto.Status, unread bool, size float32) ui.Element {
	return statusIconP(c, pal, st, unread, -1, size)
}

// statusIconP is statusIcon with a program's progress (OSC 7501), 0–100,
// which a working ring fills instead of turning; -1 for none.
func statusIconP(c *ui.Context, pal *palette, st proto.Status, unread bool, progress int, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0)
	switch {
	case st == proto.Working:
		box.Label(locales.StatusWorking())
	case st == proto.Waiting:
		box.Label(locales.StatusNeedsYou())
	case st == proto.Exited:
		box.Label(locales.StatusExited())
	case st == proto.Failed:
		box.Label(locales.StatusFailed())
	case unread:
		box.Label(locales.StatusDone())
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		rad := min(r.W, r.H)/2 - 1
		switch {
		case st == proto.Working && progress >= 0:
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.7, pal.text.Alpha(0.13))
			// From the top, clockwise: the circle starts at 3 o'clock.
			p.Transform(ui.Rotation(-90), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, max(0.02, float32(progress)/100)), 1.7, pal.text)
			})
		case st == proto.Working:
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.7, pal.text.Alpha(0.13))
			p.AnimationFrame()
			deg := float32(p.Now().UnixMilli()%1000) / 1000 * 360
			p.Transform(ui.Rotation(deg), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, 0.3), 1.7, pal.text)
			})
		case st == proto.Waiting:
			var ring ui.Path
			ring.Circle(cx, cy, rad)
			p.StrokePath(&ring, 1.7, pal.attention)
			p.AnimationFrame()
			pulse := float32(0.5 + 0.5*math.Sin(float64(p.Now().UnixMilli()%1600)/1600*2*math.Pi))
			var dot ui.Path
			dot.Circle(cx, cy, rad*0.42)
			p.FillPath(&dot, pal.attention.Alpha(0.65+0.35*pulse))
		case st == proto.Failed:
			var disc ui.Path
			disc.Circle(cx, cy, rad+0.5)
			col := pal.danger
			if !unread {
				col = pal.danger.Alpha(0.75)
			}
			p.FillPath(&disc, col)
			p.Line(cx, cy-rad*0.45, cx, cy+rad*0.08, 1.7, pal.onDone)
			var dot ui.Path
			dot.Circle(cx, cy+rad*0.42, 1)
			p.FillPath(&dot, pal.onDone)
		case st == proto.Exited:
			col := pal.textFaint
			if unread {
				col = pal.danger
			}
			var ring ui.Path
			ring.Circle(cx, cy, rad)
			p.StrokePath(&ring, 1.5, col)
			k := rad * 0.38
			p.Line(cx-k, cy-k, cx+k, cy+k, 1.5, col)
			p.Line(cx+k, cy-k, cx-k, cy+k, 1.5, col)
		case unread:
			var disc ui.Path
			disc.Circle(cx, cy, rad+0.5)
			p.FillPath(&disc, pal.done)
			check := (&ui.Path{}).MoveTo(cx-rad*0.42, cy+0.2).LineTo(cx-rad*0.08, cy+rad*0.36).LineTo(cx+rad*0.46, cy-rad*0.34)
			p.StrokePath(check, 1.6, pal.onDone)
		default:
			check := (&ui.Path{}).MoveTo(cx-rad*0.62, cy+0.4).LineTo(cx-rad*0.12, cy+rad*0.5).LineTo(cx+rad*0.66, cy-rad*0.46)
			p.StrokePath(check, 1.5, pal.textFaint)
		}
	})
	return box
}

// --- the card ---

func (a *App) card(c *ui.Context, pal *palette, bar ui.TitleBar, shown, sideShown float32) {
	s := a.session(a.selected)
	h := a.selectedHost()
	g, hasProject := a.currentGroup()
	if hasProject && a.loaded && (!a.prefs.SSHList || len(g.sessions) > 0) {
		a.sessionTabs(c, pal, bar, shown, g)
		ui.Box(c.Key("tabs-gap")).Height(cardGap / 2).Shrink(0)
	}
	if s != nil && h.conn != connVersion && a.loaded {
		// 容器在面板收起时也保留，展开首帧即可读取已提交的宽度，避免先用默认宽度再收缩。
		row := ui.Row(c.Key("with-side")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Bind(a.paneRef("with-side"))
		w := a.paneRef("with-side").Bounds(c).W
		row.Children(func() {
			ui.Column(c.Key("panes")).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
				a.panes(c, pal, bar, shown, s, a.side.visible == "")
			})
			if a.side.visible != "" {
				width := a.sideWidth(w)
				// 只展开外层裁剪区域，内容保持最终宽度，避免动画中反复换行。
				ui.Row(c.Key("side-reveal")).Width((width + paneGap) * sideShown).Shrink(0).
					MinWidth(0).AlignItems(ui.Stretch).ClipX().Disabled(a.prefs.Side == "").Bind(a.paneRef("side-reveal")).Children(func() {
					a.sideEdge(c, pal, w)
					a.sidePanel(c, pal, s, width, !hasProject, a.side.visible)
				})
			}
		})
		return
	}
	if a.prefs.SSHList && s == nil {
		ui.Column(c.Key("ssh-empty")).Grow(1).MinHeight(0).Radius(14).Background(pal.card).Clip().DragWindow().Children(func() {
			ui.Row(c).Height(tabBarH).Shrink(0).AlignItems(ui.Center).Padding(0, 10).Children(func() {
				ui.Spacer(c)
				windowControls(c, pal, a.win)
			})
			ui.Column(c.Key("ssh-empty-message")).Grow(1).Center().Gap(14).Padding(24).Children(func() {
				message := "此连接尚未打开终端"
				if a.prefs.SSHBlank {
					message = "点击下方按钮打开 SSH 终端"
				}
				if a.prefs.SSHConnection == "" {
					message = "请选择左侧的 SSH 连接"
				}
				ui.Text(c, message).FontSize(13).TextColor(pal.textFaint)
				if cfg := a.set.Host(a.prefs.SSHConnection); cfg != nil {
					connection := *cfg
					label := "打开 SSH 终端"
					if a.sshOpening[connection.Name] {
						label = "正在连接…"
					}
					ui.PrimaryButton(c.Key("open-ssh-terminal"), label).Disabled(a.sshOpening[connection.Name]).OnClick(func() {
						a.defer_(func() { a.openSSHTerminal(connection) })
					})
				}
			})
		})
		return
	}
	if a.side.visible == sideSystem {
		ui.Row(c.Key("system-standalone")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Spacer(c)
			ui.Row(c.Key("side-reveal")).Width(defaultSide * sideShown).Shrink(0).
				MinWidth(0).AlignItems(ui.Stretch).ClipX().Children(func() {
				a.sidePanel(c, pal, s, defaultSide, !hasProject, a.side.visible)
			})
		})
		return
	}
	// No session: one card, for what the app has to say.
	card := paned(ui.Column(c).Grow(1).MinHeight(0).Radius(14).Background(pal.card), pal).Clip()
	card.Children(func() {
		if !hasProject {
			left := a.chrome.headerPad
			ui.Row(c).Height(tabBarH).Shrink(0).AlignItems(ui.Center).Padding(0, 10, 0, left).DragWindow().Children(func() {
				ui.Text(c, "PaneNest").FontSize(14).FontWeight(600).TextColor(pal.text)
				ui.Spacer(c)
				windowControls(c, pal, a.win)
			})
		}
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			switch {
			case h.conn == connVersion && h.remote():
				a.message(c, pal, locales.HostVersionTitle(h.name), a.hostVersionDetail(h), locales.HostUpdateAction(h.name), func() { a.offerHostUpdate(h) })
			case h.conn == connVersion:
				a.message(c, pal, locales.UpdatedTitle(), locales.UpdatedDetail(), locales.CommonRestartService(), a.restartDaemon)
			case h.conn == connFailed && h.cl == nil && !h.remote():
				a.message(c, pal, locales.ConnCantReach(), h.err, locales.CommonTryAgain(), func() { h.retries = 0; h.conn = connConnecting; a.connect() })
			case !a.loaded:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Spinner(c).Label(locales.StatusConnectingLabel())
				})
			default:
				a.emptyState(c, pal)
			}
		})
	})
}

func (a *App) message(c *ui.Context, pal *palette, title, detail, action string, fn func()) {
	ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(600).TextColor(pal.text)
		ui.Text(c, detail).FontSize(13).TextColor(pal.textMuted).MaxWidth(420).TextAlign(ui.Center).LineHeight(1.4)
		if action != "" {
			ui.Box(c).Height(8)
			if button(c, "msg-action", action, "", true, pal) {
				fn()
			}
		}
	})
}

func (a *App) emptyState(c *ui.Context, pal *palette) {
	ui.Column(c).Fill().Center().Gap(0).Padding(24).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(pal.chip).TextColor(pal.textMuted).Margin(0, 0, 18, 0).Children(func() {
			ui.Icon(c, terminalIcon).Size(26, 26)
		})
		ui.Text(c, locales.EmptyTitle()).FontSize(19).FontWeight(650).TextColor(pal.text)
		ui.Text(c, locales.EmptyDetail()).
			FontSize(13.5).TextColor(pal.textMuted).MaxWidth(380).TextAlign(ui.Center).LineHeight(1.45).Margin(8, 0, 22, 0)
		ui.Row(c).Gap(10).Children(func() {
			if button(c, "empty-new", locales.ActionNewSession(), a.keys("session.new"), true, pal) {
				a.newSession("")
			}
			if button(c, "empty-open", locales.ActionOpenProject(), a.keys("project.open"), false, pal) {
				a.openProject()
			}
		})
		ui.Column(c).Margin(40, 0, 0, 0).Padding(14, 16).Gap(6).Radius(10).Background(pal.chip).MaxWidth(420).Children(func() {
			ui.Text(c, locales.EmptyForAgents()).FontSize(11.5).FontWeight(600).TextColor(pal.textMuted)
			ui.Text(c, "panenest new -n review -- claude\npanenest prompt review \"run the tests and fix them\"\npanenest wait review && panenest read review").
				Font("monospace").FontSize(12).TextColor(pal.text).LineHeight(1.5).Selectable()
		})
	})
}
