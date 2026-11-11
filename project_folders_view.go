package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"panenest/internal/proto"
)

// projectFolderEdit 保存文件夹名称草稿；新建时仅在确认后移动指定项目。
type projectFolderEdit struct {
	id, draft, project string
	creating, move     bool
	frames             int
}

var newFolderIcon = icon(`<path d="M9 13H3a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h4l2 2h4a1 1 0 0 1 1 1v2M12 10v5M9.5 12.5h5"/>`)

// prepareFolderNameEdit 先结束已有名称编辑，避免旧输入失焦后抢走新输入的焦点。
func (a *App) prepareFolderNameEdit() {
	if a.renaming != "" {
		a.commitRename()
	}
	if a.renamingProject != "" {
		a.commitProjectRename()
	}
	if a.folderEdit.id != "" {
		a.finishProjectFolderEdit(false)
	}
}

// newProjectFolder 新建可立即改名的分组；move 表示确认后移入 project。
func (a *App) newProjectFolder(project string, move bool) {
	a.prepareFolderNameEdit()
	name := "新建分组"
	for n := 2; ; n++ {
		exists := false
		for _, f := range a.prefs.ProjectFolders {
			exists = exists || f.Name == name
		}
		if !exists {
			break
		}
		name = fmt.Sprintf("新建分组 %d", n)
	}
	id := a.createProjectFolder(name)
	a.folderEdit = projectFolderEdit{id: id, draft: name, creating: true, project: project, move: move}
}

// startProjectFolderEdit 开始编辑已有分组，取消时保留原名。
func (a *App) startProjectFolderEdit(id string) {
	a.prepareFolderNameEdit()
	if f := a.projectFolderByID(id); f != nil {
		a.folderEdit = projectFolderEdit{id: id, draft: f.Name}
	}
}

// finishProjectFolderEdit 提交非空名称；取消或空名称会撤销尚未完成的新分组。
func (a *App) finishProjectFolderEdit(cancel bool) {
	edit := a.folderEdit
	if edit.id == "" {
		return
	}
	a.folderEdit = projectFolderEdit{}
	name := strings.TrimSpace(edit.draft)
	if cancel || name == "" {
		if edit.creating {
			a.deleteProjectFolder(edit.id)
		}
	} else {
		a.renameProjectFolder(edit.id, name)
		if edit.move {
			a.moveProjectToFolder(edit.project, edit.id)
		}
	}
	a.focusTerminal()
}

// projectList 按文件夹和组内顺序绘制项目，主机故障提示始终位于折叠区之外。
func (a *App) projectList(c *ui.Context, pal *palette) {
	ui.Row(c.Key("project-tools")).Height(28).Padding(0, 2, 0, rowInset).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "项目").FontSize(12.5).FontWeight(600).TextColor(pal.textMuted)
		ui.Spacer(c)
		iconButton(c, "new-folder", newFolderIcon, "新建分组", pal, func() {
			a.defer_(func() { a.newProjectFolder("", false) })
		}).KeepFocus()
	})
	for _, h := range a.hosts {
		if h.remote() && a.hostNeedsRow(h) {
			a.hostLine(c, pal, h, 8)
		}
	}
	byFolder := map[string][]group{}
	for _, g := range a.groups() {
		if !g.host.remote() && !g.host.loaded {
			continue
		}
		id := a.projectFolderOf(g.key())
		byFolder[id] = append(byFolder[id], g)
	}
	seen := map[*host]bool{}
	show := func(gs []group) {
		for i, g := range gs {
			g.first = !seen[g.host]
			seen[g.host] = true
			a.groupView(c, pal, g, i)
		}
	}
	for _, folder := range a.prefs.ProjectFolders {
		a.projectFolderView(c, pal, folder, byFolder[folder.ID], show)
	}
	if len(a.prefs.ProjectFolders) > 0 && len(byFolder[""]) > 0 {
		ui.Column(c.Key("ungrouped-projects")).Margin(10, 0, 0, 0).AlignItems(ui.Stretch).Children(func() {
			hdr := ui.Row(c.Key("ungrouped")).Height(30).Padding(0, 8, 0, rowInset).Gap(6).AlignItems(ui.Center).Radius(6).Label("未分组")
			a.projectFolderDrop(hdr, pal, "")
			hdr.Children(func() {
				ui.Text(c, "未分组").FontSize(12.5).FontWeight(500).TextColor(pal.textFaint).SingleLine()
				ui.Textf(c, "%d", len(byFolder[""])).FontSize(11.5).TextColor(pal.textFaint).FontFeatures("tnum")
			})
			show(byFolder[""])
		})
	} else {
		show(byFolder[""])
	}
}

// projectFolderDrop 让标题或空分组区域接收项目，包括折叠文件夹和未分组。
func (a *App) projectFolderDrop(target ui.Element, pal *palette, id string) {
	if d, over := ui.DragOver[projectDrag](target); over && !d.folder {
		target.Background(pal.accent.Alpha(0.12))
	}
	if d, ok := ui.Drop[projectDrag](target); ok && !d.folder {
		a.defer_(func() { a.moveProjectToFolder(d.key, id) })
	}
}

// projectFolderView 绘制可折叠、改名和拖动排序的单层文件夹。
func (a *App) projectFolderView(c *ui.Context, pal *palette, folder projectFolder, gs []group, show func([]group)) {
	id := folder.ID
	ui.Column(c.Key("project-folder-"+id)).Margin(7, 0, 0, 0).AlignItems(ui.Stretch).Children(func() {
		editing := a.folderEdit.id == id
		hdr := ui.ButtonBase(c.Key("header")).Height(32).Padding(0, 8).Gap(6).AlignItems(ui.Center).Radius(6).
			Label(folder.Name).Expanded(!folder.Collapsed).KeepFocus().TextColor(pal.textMuted)
		if hdr.Hovered() {
			hdr.Background(pal.rowHover)
		}
		active, needs := false, 0
		key := a.prefs.Project
		if s := a.session(a.selected); s != nil {
			key = a.sessionProjectKey(s)
		}
		for _, g := range gs {
			active = active || g.key() == key
			for _, s := range g.sessions {
				if s.Status == proto.Waiting {
					needs++
				}
			}
		}
		if folder.Collapsed && active {
			hdr.Background(pal.rowHover).TextColor(pal.text)
		}
		if !editing {
			hdr.Drag(projectDrag{key: id, folder: true})
			if hdr.Dragging() {
				hdr.Opacity(0.45)
			}
			a.projectFolderDrop(hdr, pal, id)
			_, y, _ := hdr.PointerPosition()
			after := y >= hdr.Bounds().H/2
			if d, over := ui.DragOver[projectDrag](hdr); over && d.folder && d.key != id {
				hdr.DrawOver(func(p *ui.Painter, r ui.Rect) {
					if after {
						r.Y += r.H - 2
					}
					p.Fill(ui.Rect{X: r.X + 4, Y: r.Y, W: r.W - 8, H: 2}, pal.accent, 1)
				})
			}
			if d, ok := ui.Drop[projectDrag](hdr); ok && d.folder && d.key != id {
				a.defer_(func() { a.moveProjectFolder(d.key, id, after) })
			}
			setCollapsed := func(collapsed bool) {
				a.defer_(func() {
					if f := a.projectFolderByID(id); f != nil && f.Collapsed != collapsed {
						f.Collapsed = collapsed
						a.savePrefs()
					}
				})
			}
			hdr.OnClick(func() { setCollapsed(!folder.Collapsed) })
			hdr.OnShortcut(0, ui.KeyLeft, func() { setCollapsed(true) })
			hdr.OnShortcut(0, ui.KeyRight, func() { setCollapsed(false) })
			hdr.ContextMenu(func(m *ui.Menu) {
				if m.Item("重命名分组…").Chosen() {
					a.defer_(func() { a.startProjectFolderEdit(id) })
				}
				if m.Item("新建分组…").Chosen() {
					a.defer_(func() { a.newProjectFolder("", false) })
				}
				m.Separator()
				if m.Item("删除分组（保留项目）").Chosen() {
					a.defer_(func() { a.deleteProjectFolder(id) })
				}
			})
		}
		hdr.Children(func() {
			chev := ui.Icon(c, chevronIcon).Size(10, 10).Shrink(0).TextColor(pal.textFaint)
			chev.Rotate(chev.AnimateTo("turn", 90*b2f(!folder.Collapsed), ui.Snappy))
			ui.Icon(c, folderIcon).Size(14, 14).Shrink(0).TextColor(pal.textMuted)
			if editing {
				in := ui.TextInputBase(c.Key("folder-name"), &a.folderEdit.draft).Grow(1).MinWidth(0).FontSize(12.5).AutoFocus().Label("分组名称")
				if a.folderEdit.frames < 3 {
					a.folderEdit.frames++
					in.ScrollIntoView()
					c.Invalidate()
				}
				if in.Shortcut(0, ui.KeyEscape) {
					a.defer_(func() {
						if a.folderEdit.id == id {
							a.finishProjectFolderEdit(true)
						}
					})
				} else if in.Submitted() || (a.folderEdit.frames >= 3 && !in.Focused() && !in.Pressed()) {
					a.defer_(func() {
						if a.folderEdit.id == id {
							a.finishProjectFolderEdit(false)
						}
					})
				}
				return
			}
			ui.Text(c, folder.Name).FontSize(12.5).FontWeight(500).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if folder.Collapsed && active {
				ui.Box(c).Size(5, 5).Radius(3).Background(pal.accent).Shrink(0).Tooltip("当前项目位于此分组")
			}
			ui.Textf(c, "%d", len(gs)).FontSize(11.5).TextColor(pal.textFaint).FontFeatures("tnum").Shrink(0)
			if folder.Collapsed && needs > 0 {
				ui.Box(c).Size(6, 6).Radius(3).Background(pal.attention).Shrink(0).Tooltip(fmt.Sprintf("%d 个会话需要处理", needs))
			}
		})
		if !folder.Collapsed {
			body := ui.Column(c.Key("items")).AlignItems(ui.Stretch)
			a.projectFolderDrop(body, pal, id)
			body.Children(func() {
				if len(gs) == 0 {
					ui.Text(c, "将项目拖到这里").FontSize(11.5).TextColor(pal.textFaint).Padding(8, 10).SingleLine()
				}
				show(gs)
			})
		}
	})
}
