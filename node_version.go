package main

import (
	"github.com/egoist/mygo/ui"
	"panenest/internal/proto"
	"slices"
	"strings"
)

// nodeVersionDialog 编辑项目或标签页的 Node 版本；空值继承上一级。
func (a *App) nodeVersionDialog(c *ui.Context, pal *palette) {
	if !a.nodeOpen {
		return
	}
	ui.DialogBase(c.Key("node-version"), &a.nodeOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(480).Padding(20).Gap(12).Radius(14).Background(pal.popover), pal).Label("设置 Node 版本")
		ui.Text(c, "设置 Node 版本").FontSize(15).TextColor(pal.text)
		a.nodeVersionPicker(c, pal, a.hostOf(a.nodeTarget), &a.nodeDraft)
		ui.Text(c, "优先级：标签页 > 项目 > 全局默认。选择“继承默认”使用上一级。\n支持 nvm / fnm 已安装版本。空闲终端立即切换，忙碌时在程序结束后切换。").FontSize(12).TextColor(pal.textMuted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "取消").OnClick(func() { a.nodeOpen = false })
			ui.PrimaryButton(c, "保存").Disabled(a.nodeDraft != "" && !slices.Contains(a.hostOf(a.nodeTarget).nodeVersions, a.nodeDraft)).OnClick(func() {
				a.call(a.nodeTarget, a.nodeMethod, proto.NodeVersionParams{ID: a.nodeTarget, Version: strings.TrimSpace(a.nodeDraft)}, func(err error) {
					if err != nil {
						a.toast(err.Error())
						return
					}
					a.nodeOpen = false
				})
			})
		})
	})
}

// loadNodeVersions 在后台读取所属主机的版本，丢弃重连前的结果。
func (a *App) loadNodeVersions(h *host) {
	if h.cl == nil || h.nodeVersionsLoading {
		return
	}
	h.nodeVersionsClient = h.cl
	h.nodeVersionsLoading = true
	h.nodeVersionsLoaded = true
	cl := h.cl
	go func() {
		var versions []string
		err := cl.Call("node.versions", nil, &versions)
		a.update(func() {
			defer func() {
				if a.setWin != nil {
					a.setWin.Invalidate()
				}
			}()
			h.nodeVersionsLoading = false
			if h.cl != cl {
				h.nodeVersionsLoaded = false
				return
			}
			h.nodeVersionsError = ""
			if err != nil {
				h.nodeVersionsError = err.Error()
				return
			}
			h.nodeVersions = versions
		})
	}()
}

// nodeVersionPicker 仅允许选择已安装的版本或继承默认。
func (a *App) nodeVersionPicker(c *ui.Context, pal *palette, h *host, value *string) {
	if h.nodeVersionsClient != h.cl {
		h.nodeVersionsLoaded = false
	}
	if !h.nodeVersionsLoaded {
		a.loadNodeVersions(h)
	}
	options := append([]string{""}, h.nodeVersions...)
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			selector := popup(c, "installed-node", value, options, func(v string) string {
				if v == "" {
					return "继承默认"
				}
				return v
			}, 300, pal)
			field(c, selector, selector.Focused(), pal).Width(300).Justify(ui.SpaceBetween)
			ui.Button(c, "刷新").Disabled(h.nodeVersionsLoading).OnClick(func() { a.loadNodeVersions(h) })
		})
		note := ""
		switch {
		case h.cl == nil:
			note = "主机未连接，暂时无法读取版本列表"
		case h.nodeVersionsLoading:
			note = "正在读取已安装版本…"
		case h.nodeVersionsError != "":
			note = "读取失败：" + h.nodeVersionsError
		case len(h.nodeVersions) == 0:
			note = "未找到已安装版本，请先通过 nvm / fnm 安装后刷新"
		}
		if note != "" {
			ui.Text(c, note).FontSize(12).TextColor(pal.textMuted)
		}
	})
}

// nodeVersionBadge 标记当前层级显式设置的 Node 版本，完整配置可悬停查看。
func nodeVersionBadge(c *ui.Context, pal *palette, version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	label := "v" + strings.TrimPrefix(version, "v")
	if strings.ContainsAny(version, `/\\`) {
		label = "自定义"
	}
	ui.Row(c.Key("node-badge")).Height(18).Padding(0, 5).Radius(5).AlignItems(ui.Center).Shrink(0).
		Background(pal.chip).Tooltip("Node " + version + "\n空闲终端立即应用，忙碌时延后切换").Children(func() {
		ui.Text(c, label).FontSize(10.5).FontWeight(400).TextColor(pal.textMuted).SingleLine().MaxWidth(76).Ellipsis("…")
	})
}
