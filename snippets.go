package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

const sideSnippets = "snippets"

// commandSnippet 是保存在本机偏好中的命令或脚本。
type commandSnippet struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// rightRail 提供右侧面板入口，点击时保留终端的选择状态。
func (a *App) rightRail(c *ui.Context, pal *palette) {
	top := float32(cardGap)
	if _, hasProject := a.currentGroup(); hasProject && a.loaded {
		top += tabBarH + cardGap/2
	}
	ui.Column(c.Key("right-rail")).Width(32).Shrink(0).Padding(top, 0, cardGap, 0).Gap(8).Children(func() {
		b := ui.ButtonBase(c.Key("snippets")).Size(28, 28).Radius(7).Center().KeepFocus().
			Label("命令片段").Tooltip("命令片段").TextColor(pal.textMuted)
		if b.Hovered() {
			b.Background(pal.rowHover).TextColor(pal.text)
		}
		if a.prefs.Side == sideSnippets {
			b.Background(pal.rowHover).TextColor(pal.accent)
		}
		b.Children(func() { ui.Icon(c, snippetIcon).Size(18, 18) })
		b.OnClick(func() { a.defer_(func() { a.toggleSide(sideSnippets) }) })
		a.systemButton(c, pal)
	})
}

// insertSnippet 将命令粘贴到当前选中的终端，不发送回车。
func (a *App) insertSnippet(command string) {
	s := a.session(a.selected)
	if s == nil {
		a.toast("请先打开终端")
		return
	}
	id := a.focusOf(s)
	e := a.terms[id]
	if e == nil || e.term == nil {
		a.toast("当前窗格没有可用的终端")
		return
	}
	e.term.Paste(command)
	a.side.focused = false
	a.focusTerminal()
}

// openSnippet 打开新增或编辑弹窗，负索引表示新增。
func (a *App) openSnippet(index int) {
	a.snippetEdit = nil
	a.snippetName, a.snippetCommand = "", ""
	if index >= 0 && index < len(a.prefs.Snippets) {
		a.snippetEdit = &index
		a.snippetName, a.snippetCommand = a.prefs.Snippets[index].Name, a.prefs.Snippets[index].Command
	}
	a.snippetOpen = true
}

// snippetAction 构建列表中的轻量操作按钮，统一尺寸、线条与悬停反馈。
func snippetAction(c *ui.Context, key string, glyph *ui.SVG, label string, pal *palette, run func()) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(24, 24).Radius(5).Center().KeepFocus().Label(label).Tooltip(label).TextColor(pal.textMuted)
	if b.Hovered() || b.Focused() {
		b.Background(pal.text.Alpha(0.07)).TextColor(pal.text)
	}
	if b.Pressed() {
		b.Background(pal.text.Alpha(0.12))
	}
	b.Children(func() { ui.Icon(c, glyph).Size(14, 14) })
	b.OnClick(run)
	return b
}

// snippetsView 显示可搜索的片段列表，点击条目填入当前终端。
func (a *App) snippetsView(c *ui.Context, pal *palette) {
	ui.Row(c.Key("snippet-search")).Padding(4, 16, 12, 16).Children(func() {
		ui.TextInput(c.Key("query"), &a.snippetQuery).Grow(1).MinWidth(0).Placeholder("搜索片段…").Label("搜索片段").Background(pal.rowHover).Radius(6)
	})
	ui.Scroll(c.Key("snippet-scroll")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, 16, 16, 16).Gap(5).AlignItems(ui.Stretch).Children(func() {
			query := strings.ToLower(strings.TrimSpace(a.snippetQuery))
			count := 0
			for i, snippet := range a.prefs.Snippets {
				if query != "" && !strings.Contains(strings.ToLower(snippet.Name+"\n"+snippet.Command), query) {
					continue
				}
				count++
				row := ui.Row(c.Key(fmt.Sprint(i))).AlignItems(ui.Stretch).Radius(6).Background(pal.rowHover.Alpha(0.65)).Clip()
				hover := row.Hovered() || row.FocusWithin()
				if hover {
					row.Background(pal.rowHover)
				}
				row.Children(func() {
					ui.ButtonBase(c.Key("insert")).Grow(1).MinWidth(0).Padding(9, 66, 9, 10).AlignItems(ui.Stretch).KeepFocus().Label(snippet.Name).Tooltip("填入终端：" + snippet.Command).OnClick(func() { a.insertSnippet(snippet.Command) }).Children(func() {
						ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Start).Children(func() {
							ui.Icon(c, copyIcon).Size(12, 12).Margin(2, 0, 0, 0).Shrink(0).TextColor(pal.textFaint)
							ui.Column(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Stretch).Children(func() {
								ui.Text(c, snippet.Name).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…")
								preview := strings.SplitN(snippet.Command, "\n", 3)
								if len(preview) > 2 {
									preview = preview[:2]
								}
								for _, line := range preview {
									ui.Text(c, line).Font(a.termFamily()).FontSize(11).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
								}
							})
						})
					})
					if hover {
						ui.Row(c.Key("actions")).Absolute().Right(6).Top(7).Gap(2).AlignItems(ui.Center).Children(func() {
							snippetAction(c, "copy", snippetCopyIcon, "复制指令", pal, func() { c.WriteClipboard(snippet.Command); a.toast("已复制指令") }).KeepFocus()
							snippetAction(c, "edit", snippetEditIcon, "编辑片段", pal, func() { a.openSnippet(i) }).KeepFocus()
						})
					}
				})
			}
			if count == 0 {
				message := "还没有命令片段，点击右上角 + 新增。"
				if query != "" {
					message = "没有匹配的片段"
				}
				ui.Column(c).Padding(32, 12).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(14).Background(pal.accent.Alpha(0.07)).Center().Children(func() { ui.Icon(c, terminalIcon).Size(24, 24).TextColor(pal.accent) })
					ui.Text(c, message).FontSize(12).TextColor(pal.textFaint)
				})
			}
		})
	})
}

// snippetDialog 新增或编辑片段；名称及内容必填，编辑时提供删除入口。
func (a *App) snippetDialog(c *ui.Context, pal *palette) {
	if !a.snippetOpen {
		return
	}
	title, saveLabel := "创建片段", "创建片段"
	if a.snippetEdit != nil {
		title, saveLabel = "编辑片段", "保存修改"
	}
	if ui.DialogBase(c.Key("create-snippet"), &a.snippetOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(520).Padding(20).Gap(10).Radius(14).Background(pal.popover), pal).Label(title)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(pal.text)
			ui.Spacer(c)
			iconButton(c, "close", closeIcon, "关闭", pal, func() { a.snippetOpen = false })
		})
		ui.Text(c, "名称 *").FontSize(13).TextColor(pal.text)
		ui.TextInput(c.Key("name"), &a.snippetName).Placeholder("例如：Docker 容器清理").Label("片段名称").Background(pal.rowHover).Radius(6).AutoFocus()
		ui.Text(c, "内容 *").FontSize(13).TextColor(pal.text)
		ui.TextArea(c.Key("content"), &a.snippetCommand).Height(180).Placeholder("例如：docker system prune -af").Label("片段内容").Background(pal.rowHover).Radius(6)
		ui.Text(c, "可以是命令、脚本或代码片段").FontSize(12).TextColor(pal.textFaint)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if a.snippetEdit != nil {
				ui.Button(c.Key("delete"), "删除片段").TextColor(pal.danger).OnClick(func() {
					i := *a.snippetEdit
					if i >= 0 && i < len(a.prefs.Snippets) {
						a.prefs.Snippets = append(a.prefs.Snippets[:i], a.prefs.Snippets[i+1:]...)
						a.savePrefs()
					}
					a.snippetOpen = false
				})
			}
			ui.Spacer(c)
			ui.Button(c.Key("cancel"), "取消").OnClick(func() { a.snippetOpen = false })
			ui.PrimaryButton(c.Key("save"), saveLabel).Disabled(strings.TrimSpace(a.snippetName) == "" || strings.TrimSpace(a.snippetCommand) == "").OnClick(func() {
				name, command := strings.TrimSpace(a.snippetName), strings.TrimRight(a.snippetCommand, "\r\n")
				if name == "" || strings.TrimSpace(command) == "" {
					return
				}
				if strings.ContainsAny(command, "\x00\x1b") {
					a.toast("片段包含不支持的控制字符")
					return
				}
				snippet := commandSnippet{Name: name, Command: command}
				if a.snippetEdit != nil {
					i := *a.snippetEdit
					if i < 0 || i >= len(a.prefs.Snippets) {
						return
					}
					a.prefs.Snippets[i] = snippet
				} else {
					a.prefs.Snippets = append(a.prefs.Snippets, snippet)
				}
				a.savePrefs()
				a.snippetOpen = false
				a.snippetQuery = ""
			})
		})
	}).Dismissed() {
		// 保留输入内容，只允许通过弹窗按钮关闭，避免遮罩或 Escape 误触。
		a.snippetOpen = true
	}
}
