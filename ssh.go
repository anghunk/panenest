package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"panenest/internal/proto"
	"panenest/internal/remote"
	"panenest/internal/settings"
)

// sshConnectionDraft 保存弹窗输入，密码随连接保存到本地配置。
type sshConnectionDraft struct {
	hostDraft
	user, port, password string
	showPassword         bool
}

// sshSessionName 从启动命令识别 SSH 会话，不依赖可修改的标签名。
func sshSessionName(s *proto.Session) string {
	if s == nil {
		return ""
	}
	for _, p := range append([]proto.Pane{s.Pane}, s.Panes...) {
		if len(p.Command) == 4 && p.Command[1] == "__ssh-terminal" {
			return p.Command[3]
		}
	}
	return ""
}

// toggleSSHList 在两个模式之间恢复各自上次选择的会话。
func (a *App) toggleSSHList() {
	if s := a.session(a.selected); s != nil {
		if sshSessionName(s) != "" {
			a.prefs.SSHSelected = s.ID
		} else {
			a.prefs.TerminalSelected = s.ID
			a.prefs.TerminalProject = a.prefs.Project
		}
	}
	a.prefs.SSHList = !a.prefs.SSHList
	if a.prefs.SSHList && a.prefs.SSHConnection != "" {
		a.selectSSHConnection(settings.Host{Name: a.prefs.SSHConnection})
		return
	}
	target := a.prefs.TerminalSelected
	if a.prefs.SSHList {
		target = a.prefs.SSHSelected
	} else {
		a.prefs.Project = a.prefs.TerminalProject
	}
	a.selected = ""
	if s := a.session(target); s != nil && (sshSessionName(s) != "") == a.prefs.SSHList {
		a.selectSession(target)
	} else {
		for i := range a.st.Sessions {
			s := &a.st.Sessions[i]
			if (sshSessionName(s) != "") == a.prefs.SSHList {
				a.selectSession(s.ID)
				break
			}
		}
	}
	a.reportView()
	a.updateTitle()
	a.savePrefs()
}

// newSSHBlank 打开当前 SSH 连接的空白页，等待用户主动连接。
func (a *App) newSSHBlank() {
	a.prefs.SSHBlank = true
	a.selected, a.prefs.SSHSelected = "", ""
	a.reportView()
	a.updateTitle()
	a.savePrefs()
}

// selectSSHConnection 仅恢复所选连接自己的终端；没有终端时保留空白。
func (a *App) selectSSHConnection(cfg settings.Host) {
	a.prefs.SSHList = true
	a.prefs.SSHBlank = false
	a.prefs.SSHConnection = cfg.Name
	a.selected, a.prefs.SSHSelected = "", ""
	remembered := a.prefs.ProjectSessions["ssh:"+cfg.Name]
	if s := a.session(remembered); s != nil && sshSessionName(s) == cfg.Name {
		a.selectSession(s.ID)
		return
	}
	for i := len(a.st.Sessions) - 1; i >= 0; i-- {
		if sshSessionName(&a.st.Sessions[i]) == cfg.Name {
			a.selectSession(a.st.Sessions[i].ID)
			return
		}
	}
	a.reportView()
	a.updateTitle()
	a.savePrefs()
}

// sshConnectionDrag 区分 SSH 连接排序与项目、终端标签拖拽。
type sshConnectionDrag struct{ name string }

// moveSSHConnection 调整 SSH 连接顺序，并保持其他类型主机在配置中的位置。
func (a *App) moveSSHConnection(name, target string, after bool) {
	s := a.set.Clone()
	var slots []int
	var connections []settings.Host
	for i, cfg := range s.Hosts {
		addr, err := remote.ParseAddr(cfg.Addr)
		if err == nil && addr.Scheme == "ssh" {
			slots = append(slots, i)
			connections = append(connections, cfg)
		}
	}
	from := slices.IndexFunc(connections, func(h settings.Host) bool { return h.Name == name })
	to := slices.IndexFunc(connections, func(h settings.Host) bool { return h.Name == target })
	if from < 0 || to < 0 || from == to {
		return
	}
	if after {
		to++
	}
	if from < to {
		to--
	}
	if from == to {
		return
	}
	connection := connections[from]
	connections = slices.Delete(connections, from, from+1)
	connections = slices.Insert(connections, to, connection)
	for i, slot := range slots {
		s.Hosts[slot] = connections[i]
	}
	a.applySettings(s)
}

// sshSidebar 显示本地保存的 SSH 连接，点击后恢复该连接的终端。
func (a *App) sshSidebar(c *ui.Context, pal *palette) {
	ui.Scroll(c.Key("ssh-list")).Grow(1).MinHeight(0).Padding(8, 10, 16, 10).Children(func() {
		ui.Column(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
			count := 0
			for _, cfg := range a.set.Hosts {
				addr, err := remote.ParseAddr(cfg.Addr)
				if err != nil || addr.Scheme != "ssh" {
					continue
				}
				count++
				selected := a.prefs.SSHConnection == cfg.Name
				row := ui.Row(c.Key("ssh-" + cfg.Name)).Gap(6).AlignItems(ui.Center).Padding(8).Radius(8)
				if pal.dark {
					row.Border(1, ui.Transparent)
				}
				bg := ui.Transparent
				if selected {
					bg = pal.rowSelected
					chosen(row, pal)
				} else if row.Hovered() {
					bg = pal.rowHover
				}
				row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
				row.Drag(sshConnectionDrag{cfg.Name})
				if row.Dragging() {
					row.Opacity(0.45)
				}
				_, y, _ := row.PointerPosition()
				after := y >= row.Bounds().H/2
				if d, over := ui.DragOver[sshConnectionDrag](row); over && d.name != cfg.Name {
					row.DrawOver(func(p *ui.Painter, r ui.Rect) {
						if after {
							r.Y += r.H - 2
						}
						p.Fill(ui.Rect{X: r.X + 4, Y: r.Y, W: r.W - 8, H: 2}, pal.accent, 1)
					})
				}
				if d, ok := ui.Drop[sshConnectionDrag](row); ok && d.name != cfg.Name {
					a.defer_(func() { a.moveSSHConnection(d.name, cfg.Name, after) })
				}
				row.ContextMenu(func(m *ui.Menu) {
					if m.Item("连接").Disabled(a.sshOpening[cfg.Name]).Chosen() {
						a.defer_(func() { a.openSSHTerminal(cfg) })
					}
					m.Separator()
					if m.Item("修改连接与认证").Chosen() {
						a.sshDraft = sshConnectionDraft{hostDraft: hostDraft{name: cfg.Name, addr: addr.Host, editing: cfg.Name}, user: addr.User, port: addr.Port, password: cfg.Password}
						a.sshAdding = true
					}
					m.Separator()
					if m.Item("删除 SSH 连接").Chosen() {
						a.confirm = &confirmation{
							open:    true,
							title:   "删除 SSH 连接？",
							detail:  "确定删除连接“" + cfg.Name + "”及保存的账号密码？已打开的终端会保留。",
							action:  "删除连接",
							destroy: true,
							run:     func() { a.removeHost(cfg.Name) },
						}
					}
				}).Children(func() {
					ui.ButtonBase(c.Key("connect")).Grow(1).MinWidth(0).Padding(0).Justify(ui.Start).AlignItems(ui.Stretch).Label("连接 " + cfg.Name).Background(ui.Transparent).OnClick(func() { a.defer_(func() { a.selectSSHConnection(cfg) }) }).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Stretch).Children(func() {
							ui.Text(c, cfg.Name).FontSize(13).FontWeight(600).TextColor(pal.text)
							ui.Text(c, addr.String()).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
						})
					})
					if a.sshOpening[cfg.Name] {
						ui.Spinner(c).Size(16, 16).Tooltip("正在连接…")
					}
				})
			}
			if count == 0 {
				ui.Text(c, "还没有 SSH 连接\n点击 + 新建连接").FontSize(12.5).TextColor(pal.textFaint).Padding(12, 8)
			}
		})
	})
}

// openSSHTerminal 先异步校验 SSH 登录，成功后才创建本地终端标签。
func (a *App) openSSHTerminal(cfg settings.Host) {
	if a.local().cl == nil {
		a.toast("本机终端服务尚未就绪，请稍后重试")
		return
	}
	if a.sshOpening[cfg.Name] {
		return
	}
	if a.sshOpening == nil {
		a.sshOpening = map[string]bool{}
	}
	a.sshOpening[cfg.Name] = true
	if !a.prefs.SSHList || a.prefs.SSHConnection != cfg.Name {
		a.selectSSHConnection(cfg)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		params, err := prepareSSHTerminal(ctx, cfg)
		a.update(func() {
			delete(a.sshOpening, cfg.Name)
			current := a.set.Host(cfg.Name)
			if current == nil || *current != cfg {
				return
			}
			if err != nil {
				a.toast("SSH 连接失败：" + err.Error())
				return
			}
			if !a.prefs.SSHList || a.prefs.SSHConnection != cfg.Name {
				return
			}
			a.create(a.local(), params)
		})
	}()
}

// prepareSSHTerminal 验证网络与认证，失败时不返回可创建的终端参数。
func prepareSSHTerminal(ctx context.Context, cfg settings.Host) (proto.CreateParams, error) {
	cmd, err := sshTerminalCommand(ctx, cfg, false)
	if err != nil {
		return proto.CreateParams{}, err
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return proto.CreateParams{}, fmt.Errorf("连接超时或已取消")
		}
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		if len(detail) > 1000 {
			detail = detail[:1000]
		}
		return proto.CreateParams{}, errors.New(detail)
	}
	exe, err := os.Executable()
	if err != nil {
		return proto.CreateParams{}, err
	}
	return proto.CreateParams{Name: "SSH · " + cfg.Name, Dir: proto.Home(), Command: []string{exe, "__ssh-terminal", settings.Path(), cfg.Name}}, nil
}

// sshTerminalCommand 统一连接校验与交互终端的地址、主机指纹和密码认证参数。
func sshTerminalCommand(ctx context.Context, cfg settings.Host, interactive bool) (*exec.Cmd, error) {
	addr, err := remote.ParseAddr(cfg.Addr)
	if err != nil || addr.Scheme != "ssh" {
		return nil, errors.New("无效的 SSH 地址")
	}
	tty := "-T"
	if interactive {
		tty = "-tt"
	}
	args := []string{tty, "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3"}
	if !interactive && cfg.Password == "" {
		args = append(args, "-o", "BatchMode=yes")
	}
	if cfg.Password != "" {
		args = append(args, "-o", "NumberOfPasswordPrompts=1")
	}
	if addr.Port != "" {
		args = append(args, "-p", addr.Port)
	}
	if addr.User != "" {
		args = append(args, "-l", addr.User)
	}
	args = append(args, "--", addr.Host)
	if !interactive {
		args = append(args, "exit 0")
	}
	cmd := exec.CommandContext(ctx, remote.SSHCommand(), args...)
	cmd.WaitDelay = time.Second
	if cfg.Password != "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd.Env = append(os.Environ(), "SSH_ASKPASS="+exe, "SSH_ASKPASS_REQUIRE=force", "DISPLAY=panenest:0", "PANENEST_SSH_PASSWORD="+cfg.Password, "PANENEST_SSH_ASKPASS=1")
	}
	return cmd, nil
}

// runSSHTerminal 从本地配置读取认证信息，透传终端输入输出及退出状态。
func runSSHTerminal(configPath, name string) int {
	os.Setenv("PANENEST_CONFIG", configPath)
	s := settings.Load()
	cfg := s.Host(name)
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "SSH 连接已被删除：", name)
		return 1
	}
	cmd, err := sshTerminalCommand(context.Background(), *cfg, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// openSSHDialog 打开新建连接弹窗并清空上次未保存的内容。
func (a *App) openSSHDialog() {
	a.sshDraft = sshConnectionDraft{port: "22"}
	a.sshAdding = true
}

// sshDialog 收集 SSH 地址与名称，校验失败时保留输入并显示错误。
func (a *App) sshDialog(c *ui.Context, pal *palette) {
	if !a.sshAdding {
		return
	}
	d := &a.sshDraft
	ui.DialogBase(c.Key("create-ssh"), &a.sshAdding, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(520).Padding(20).Gap(10).Radius(14).Background(pal.popover), pal).Label("新建 SSH 连接")
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "新建 SSH 连接").FontSize(15).FontWeight(600).TextColor(pal.text)
			ui.Spacer(c)
			iconButton(c, "close", closeIcon, "关闭", pal, func() { a.sshAdding = false; d.password = "" })
		})
		ui.Text(c, "IP / 主机地址 *").FontSize(13).TextColor(pal.text)
		ui.TextInput(c.Key("address"), &d.addr).Placeholder("例如：192.168.1.10，也支持 SSH 地址").Label("SSH 地址").Background(pal.rowHover).Radius(6).AutoFocus().OnSubmit(func() { a.defer_(a.addSSH) })
		ui.Text(c, "端口").FontSize(13).TextColor(pal.text)
		ui.TextInput(c.Key("port"), &d.port).Placeholder("22").Label("SSH 端口").Background(pal.rowHover).Radius(6)
		ui.Text(c, "账号").FontSize(13).TextColor(pal.text)
		ui.TextInput(c.Key("user"), &d.user).Placeholder("例如：root").Label("SSH 账号").Background(pal.rowHover).Radius(6)
		ui.Text(c, "密码（可选）").FontSize(13).TextColor(pal.text)
		ui.Row(c.Key("password-field")).Gap(4).AlignItems(ui.Center).Background(pal.rowHover).Radius(6).Padding(0, 4, 0, 0).Children(func() {
			input := ui.TextInput(c.Key("password"), &d.password).Grow(1).MinWidth(0).Placeholder("留空使用系统 SSH 密钥").Label("SSH 密码").Background(ui.Transparent).Radius(6)
			if !d.showPassword {
				input.Password()
			}
			glyph, label := eyeIcon, "显示密码"
			if d.showPassword {
				glyph, label = eyeOffIcon, "隐藏密码"
			}
			iconButton(c, "password-visibility", glyph, label, pal, func() { d.showPassword = !d.showPassword }).KeepFocus()
		})
		ui.Text(c, "连接名称（可选）").FontSize(13).TextColor(pal.text)
		ui.TextInput(c.Key("name"), &d.name).Placeholder("例如：开发服务器").Label("连接名称").Background(pal.rowHover).Radius(6).OnSubmit(func() { a.defer_(a.addSSH) })
		ui.Text(c, "连接与密码保存在本地配置；直接登录 SSH 终端。首次连接自动记录主机指纹，指纹变更时会拒绝连接。").FontSize(12).TextColor(pal.textFaint)
		if d.err != "" {
			ui.Text(c, d.err).FontSize(12).TextColor(pal.danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c.Key("cancel"), "取消").OnClick(func() { a.sshAdding = false; d.password = "" })
			ui.PrimaryButton(c.Key("save"), "保存连接").Disabled(strings.TrimSpace(d.addr) == "").OnClick(func() { a.defer_(a.addSSH) })
		})
	})
	if !a.sshAdding {
		d.password = ""
	}
}

// addSSH 校验并持久化 SSH 主机，配置同步后由既有连接流程异步连接。
func (a *App) addSSH() {
	d := &a.sshDraft
	addr := strings.TrimSpace(d.addr)
	if addr == "" {
		d.err = "请输入 SSH 地址"
		return
	}
	parsed, err := remote.ParseAddr(addr)
	if err != nil {
		d.err = err.Error()
		return
	}
	if parsed.Scheme != "ssh" {
		d.err = "请输入 SSH 地址，例如 user@host"
		return
	}
	if user := strings.TrimSpace(d.user); user != "" {
		parsed.User = user
	}
	if port := strings.TrimSpace(d.port); port != "" && (port != "22" || parsed.Port == "") {
		parsed.Port = port
	}
	if parsed.Port != "" {
		port, err := strconv.Atoi(parsed.Port)
		if err != nil || port < 1 || port > 65535 {
			d.err = "端口应为 1 到 65535"
			return
		}
	}
	addr = parsed.String()
	if _, err := remote.ParseAddr(addr); err != nil {
		d.err = err.Error()
		return
	}
	if strings.ContainsAny(d.password, "\r\n\x00") {
		d.err = "密码不能包含换行或空字符"
		return
	}
	name := strings.TrimSpace(d.name)
	if name == "" {
		name = defaultHostName(addr)
	}
	cfg, err := settings.ParseHost(name + "=" + addr)
	if err != nil {
		d.err = err.Error()
		return
	}
	if a.set.Host(cfg.Name) != nil && cfg.Name != d.editing {
		d.err = "连接名称已存在"
		return
	}
	cfg.Password = d.password
	s := a.set.Clone()
	if d.editing != "" {
		old := s.Host(d.editing)
		if old == nil {
			d.err = "连接已被删除，请重新新建"
			return
		}
		cfg.RemoteControl, cfg.Color = old.RemoteControl, old.Color
		*old = cfg
	} else {
		s.Hosts = append(s.Hosts, cfg)
	}
	a.applySettings(s)
	a.selectSSHConnection(cfg)
	a.sshAdding = false
	*d = sshConnectionDraft{}
}
