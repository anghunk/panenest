package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const sideSystem = "system"

var systemIcon = icon(`<rect x="2" y="2" width="12" height="12" rx="3"/><path d="M3.5 8h2l1.5-3 2 6 1.5-3h2"/>`)

// systemMonitor 的数据仅在 UI 线程更新；关闭面板后不再启动采样。
type systemMonitor struct {
	busy    bool
	at      time.Time
	value   *systemSample
	err     string
	history []float64
}

type systemSample struct {
	cpu                              float64
	total, used, swapTotal, swapUsed uint64
}

// systemButton 在功能栏提供系统状态入口。
func (a *App) systemButton(c *ui.Context, pal *palette) {
	b := ui.ButtonBase(c.Key("system")).Size(28, 28).Radius(7).Center().KeepFocus().Label("系统状态").Tooltip("系统状态 · 本机").TextColor(pal.textMuted)
	if b.Hovered() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	if a.prefs.Side == sideSystem {
		b.Background(pal.rowHover).TextColor(pal.accent)
	}
	b.Children(func() { ui.Icon(c, systemIcon).Size(18, 18) })
	b.OnClick(func() { a.defer_(func() { a.toggleSide(sideSystem) }) })
}

// systemView 显示本机资源快照；异步采样至多每两秒启动一次。
func (a *App) systemView(c *ui.Context, pal *palette) {
	m := &a.side.system
	if !m.busy && (m.at.IsZero() || time.Since(m.at) >= 2*time.Second) {
		m.busy = true
		go func() {
			value, err := readSystemSample()
			a.update(func() {
				m.busy, m.at = false, time.Now()
				if err != nil {
					m.err = err.Error()
					return
				}
				m.value, m.err = &value, ""
				m.history = append(m.history, value.cpu)
				if len(m.history) > 30 {
					m.history = m.history[len(m.history)-30:]
				}
			})
		}()
	}
	c.After(time.Second)
	ui.Scroll(c.Key("system-scroll")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(16).Gap(14).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(pal.accent)
				ui.Text(c, "本机资源概览").FontSize(12).FontWeight(600).TextColor(pal.textMuted)
				ui.Spacer(c)
				ui.Text(c, "每 2 秒刷新").FontSize(11).TextColor(pal.textFaint)
			})
			if m.err != "" {
				ui.Text(c, "采样失败："+m.err).FontSize(12).TextColor(pal.textMuted)
			}
			if m.value == nil {
				if m.err == "" {
					ui.Text(c, "正在采集系统状态…").FontSize(13).TextColor(pal.textMuted)
				}
				return
			}
			v := m.value
			systemMetric(c.Key("cpu"), pal, "CPU", "处理器总占用", v.cpu, fmt.Sprintf("%d 核心 · %s", runtime.NumCPU(), runtime.GOARCH), m.history)
			systemMetric(c.Key("memory"), pal, "内存", "已用 / 物理内存", 100*float64(v.used)/float64(v.total), systemBytes(v.used)+" / "+systemBytes(v.total), nil)
			swap := float64(0)
			if v.swapTotal > 0 {
				swap = 100 * float64(v.swapUsed) / float64(v.swapTotal)
			}
			systemMetric(c.Key("swap"), pal, "交换空间", "磁盘上的虚拟内存", swap, systemBytes(v.swapUsed)+" / "+systemBytes(v.swapTotal), nil)
			ui.Text(c, "统计范围为当前运行 Panenest 的电脑，包含所有应用。内存占用扣除可回收缓存。").FontSize(11).TextColor(pal.textFaint).LineHeight(1.5)
			ui.Text(c, "更新于 "+m.at.Format("15:04:05")).FontSize(11).TextColor(pal.textFaint)
		})
	})
}

// systemMetric 绘制圆环资源仪表和最多三十个采样点的趋势。
func systemMetric(c *ui.Context, pal *palette, title, subtitle string, percent float64, detail string, history []float64) {
	percent = min(100, max(0, percent))
	col := pal.accent
	if percent >= 85 {
		col = ui.Hex("#e28a48")
	}
	ui.Column(c).Padding(16).Gap(10).Radius(12).Background(pal.rowHover).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(650).TextColor(pal.text)
			ui.Spacer(c)
			ui.Text(c, subtitle).FontSize(11).TextColor(pal.textFaint)
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Box(c.Key("gauge")).Size(112, 112).Shrink(0).Center().Draw(func(p *ui.Painter, r ui.Rect) {
				cx, cy := r.X+r.W/2, r.Y+r.H/2
				radius := min(r.W, r.H)/2 - 6
				const stroke = float32(7)
				var track ui.Path
				track.Circle(cx, cy, radius)
				p.StrokePath(&track, stroke, col.Alpha(0.12))
				if percent <= 0 {
					return
				}
				var arc ui.Path
				arc.MoveTo(cx, cy-radius)
				steps := int(math.Ceil(percent * 1.2))
				var x, y float32
				for i := 1; i <= steps; i++ {
					angle := -math.Pi/2 + 2*math.Pi*(percent/100)*float64(i)/float64(steps)
					x, y = cx+radius*float32(math.Cos(angle)), cy+radius*float32(math.Sin(angle))
					arc.LineTo(x, y)
				}
				p.StrokePath(&arc, stroke, col)
				var caps ui.Path
				caps.Circle(cx, cy-radius, stroke/2)
				caps.Circle(x, y, stroke/2)
				p.FillPath(&caps, col)
			}).Children(func() {
				ui.Column(c).Center().Gap(2).Children(func() {
					ui.Text(c, fmt.Sprintf("%.1f%%", percent)).FontSize(22).FontWeight(650).FontFeatures("tnum").TextColor(col)
					ui.Text(c, "占用率").FontSize(10).TextColor(pal.textFaint)
				})
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
				ui.Text(c, detail).FontSize(12).FontFeatures("tnum").TextColor(pal.textMuted).LineHeight(1.5)
				status := "使用正常"
				if percent >= 85 {
					status = "占用较高"
				}
				ui.Text(c, status).FontSize(11).TextColor(col)
			})
		})
		if history != nil {
			ui.Box(c).Height(48).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Line(r.X, r.Y+r.H, r.X+r.W, r.Y+r.H, 1, pal.edge)
				for i := 1; i < len(history); i++ {
					x := r.X + r.W*float32(i-1)/29
					y := r.Y + r.H*(1-float32(history[i-1])/100)
					p.Line(x, y, r.X+r.W*float32(i)/29, r.Y+r.H*(1-float32(history[i])/100), 1.8, col)
				}
			})
		}
	})
}

// systemBytes 使用二进制容量单位显示字节数。
func systemBytes(n uint64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%.0f KiB", float64(n)/1024)
	}
	if n < 1<<30 {
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
}

// systemCommand 限制系统命令的运行时间，防止后台采样挂起。
func systemCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// readSystemSample 通过系统自带接口读取快照，不在 UI 线程执行。
func readSystemSample() (systemSample, error) {
	var v systemSample
	switch runtime.GOOS {
	case "darwin":
		total, err := systemCommand("/usr/sbin/sysctl", "-n", "hw.memsize")
		if err != nil {
			return v, err
		}
		v.total, _ = strconv.ParseUint(strings.TrimSpace(total), 10, 64)
		vm, err := systemCommand("/usr/bin/vm_stat")
		if err != nil {
			return v, err
		}
		page := uint64(4096)
		fields := strings.Fields(strings.SplitN(vm, "\n", 2)[0])
		for i, f := range fields {
			if f == "of" && i+1 < len(fields) {
				page, _ = strconv.ParseUint(fields[i+1], 10, 64)
			}
		}
		var reclaim uint64
		for _, line := range strings.Split(vm, "\n") {
			key, val, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			if key == "Pages free" || key == "Pages inactive" || key == "Pages speculative" {
				n, _ := strconv.ParseUint(strings.Trim(strings.TrimSpace(val), "."), 10, 64)
				reclaim += n * page
			}
		}
		v.used = v.total - min(v.total, reclaim)
		swap, err := systemCommand("/usr/sbin/sysctl", "-n", "vm.swapusage")
		if err != nil {
			return v, err
		}
		sf := strings.Fields(swap)
		for i, f := range sf {
			if (f == "total" || f == "used") && i+2 < len(sf) {
				n, _ := strconv.ParseFloat(strings.TrimSuffix(sf[i+2], "M"), 64)
				if f == "total" {
					v.swapTotal = uint64(n * (1 << 20))
				} else {
					v.swapUsed = uint64(n * (1 << 20))
				}
			}
		}
		top, err := systemCommand("/usr/bin/top", "-l", "2", "-s", "1", "-n", "0")
		if err != nil {
			return v, err
		}
		found := false
		for _, line := range strings.Split(top, "\n") {
			if strings.HasPrefix(line, "CPU usage:") {
				for i, f := range strings.Fields(line) {
					if f == "idle" && i > 0 {
						n, e := strconv.ParseFloat(strings.TrimSuffix(strings.Fields(line)[i-1], "%"), 64)
						if e == nil {
							v.cpu = 100 - n
							found = true
						}
					}
				}
			}
		}
		if !found {
			return v, fmt.Errorf("无法解析 CPU 采样")
		}
	case "linux":
		t0, i0, err := systemLinuxCPU()
		if err != nil {
			return v, err
		}
		time.Sleep(time.Second)
		t1, i1, err := systemLinuxCPU()
		if err != nil {
			return v, err
		}
		if t1 > t0 {
			v.cpu = 100 * (1 - float64(i1-i0)/float64(t1-t0))
		}
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return v, err
		}
		values := map[string]uint64{}
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				n, _ := strconv.ParseUint(f[1], 10, 64)
				values[strings.TrimSuffix(f[0], ":")] = n * 1024
			}
		}
		v.total = values["MemTotal"]
		v.used = v.total - min(v.total, values["MemAvailable"])
		v.swapTotal = values["SwapTotal"]
		v.swapUsed = v.swapTotal - min(v.swapTotal, values["SwapFree"])
	default:
		return v, fmt.Errorf("当前系统暂不支持资源采样")
	}
	if v.total == 0 {
		return v, fmt.Errorf("无法读取物理内存容量")
	}
	return v, nil
}

// systemLinuxCPU 读取累计 CPU 时钟，忽略已经包含在用户时间中的 guest 字段。
func systemLinuxCPU() (total, idle uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(strings.SplitN(string(data), "\n", 2)[0])
	if len(fields) < 5 {
		return 0, 0, fmt.Errorf("无法解析 CPU 采样")
	}
	for i, f := range fields[1:] {
		if i >= 8 {
			break
		}
		n, e := strconv.ParseUint(f, 10, 64)
		if e != nil {
			return 0, 0, e
		}
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle, nil
}
