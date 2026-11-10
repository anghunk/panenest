package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"panenest/internal/proto"
	"panenest/internal/settings"
)

// configurationBackup 是可迁移的单文件配置，不携带终端内容、命令或会话历史。
type configurationBackup struct {
	Format        string                 `json:"format"`
	Version       int                    `json:"version"`
	ExportedAt    string                 `json:"exportedAt"`
	Configuration string                 `json:"configuration"`
	Workspace     proto.WorkspaceRecords `json:"workspace"`
}

// decodeConfigurationBackup 验证文件格式、配置和大小，拒绝不完整或不兼容的文件。
func decodeConfigurationBackup(r io.Reader) (configurationBackup, settings.Settings, error) {
	var out configurationBackup
	data, err := io.ReadAll(io.LimitReader(r, 16<<20+1))
	if err != nil {
		return out, settings.Settings{}, err
	}
	if len(data) > 16<<20 {
		return out, settings.Settings{}, fmt.Errorf("配置文件不能超过 16 MB")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, settings.Settings{}, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return out, settings.Settings{}, fmt.Errorf("配置文件包含多余内容")
	}
	if out.Format != "panenest-configuration" || out.Version != 1 || out.Configuration == "" {
		return out, settings.Settings{}, fmt.Errorf("不是受支持的 PaneNest 配置文件")
	}
	s, err := settings.ParseBackup(out.Configuration)
	return out, s, err
}

// backupResult 将后台操作结果发布到设置窗口。
func (a *App) backupResult(message string) {
	a.update(func() {
		a.ss.backupBusy = false
		a.ss.toast = message
		if a.setWin != nil {
			a.setWin.Invalidate()
		}
	})
}

// exportConfiguration 通过系统保存窗口导出当前设置和本机工作区元数据。
func (a *App) exportConfiguration() {
	if a.ss.backupBusy || a.local().cl == nil {
		return
	}
	a.ss.backupBusy = true
	win, cl := a.setWin, a.local().cl
	config := settings.Reference(a.set.Clone())
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "导出配置", DefaultPath: "panenest-configuration.json", Filters: []mygo.FileFilter{{Name: "PaneNest 配置", Extensions: []string{"json"}}}})
		if err != nil {
			a.backupResult("导出失败：" + err.Error())
			return
		}
		if path == "" {
			a.backupResult("")
			return
		}
		backup := configurationBackup{Format: "panenest-configuration", Version: 1, ExportedAt: time.Now().Format(time.RFC3339), Configuration: config}
		if err = cl.Call("workspace.export", nil, &backup.Workspace); err != nil {
			a.backupResult("导出失败：" + err.Error())
			return
		}
		data, err := json.MarshalIndent(backup, "", "  ")
		if err == nil {
			err = writeConfigurationBackup(path, append(data, '\n'))
		}
		if err != nil {
			a.backupResult("导出失败：" + err.Error())
			return
		}
		a.backupResult("配置已导出")
	}()
}

// importConfiguration 读取并校验文件，在展示导入范围后合并本机记录并应用设置。
func (a *App) importConfiguration() {
	if a.ss.backupBusy || a.local().cl == nil {
		return
	}
	a.ss.backupBusy = true
	win, cl := a.setWin, a.local().cl
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "导入配置", Filters: []mygo.FileFilter{{Name: "PaneNest 配置", Extensions: []string{"json"}}}})
		if err != nil {
			a.backupResult("导入失败：" + err.Error())
			return
		}
		if len(paths) == 0 {
			a.backupResult("")
			return
		}
		f, err := os.Open(paths[0])
		if err != nil {
			a.backupResult("导入失败：" + err.Error())
			return
		}
		backup, config, err := decodeConfigurationBackup(f)
		f.Close()
		if err != nil {
			a.backupResult("导入失败：" + err.Error())
			return
		}
		a.update(func() {
			a.ss.backupBusy = false
			if a.setWin != win || a.local().cl != cl {
				return
			}
			a.ss.confirm = &confirmation{open: true, title: "导入配置", action: "导入", detail: fmt.Sprintf("将合并 %d 个项目和 %d 个标签页，并应用文件中的设置。已有终端保持运行，导入的终端标签页需手动重启。", len(backup.Workspace.Projects), len(backup.Workspace.Tabs)), run: func() {
				if a.ss.backupBusy {
					return
				}
				a.ss.backupBusy = true
				go func() {
					var added int
					if err := cl.Call("workspace.import", backup.Workspace, &added); err != nil {
						a.backupResult("导入失败：" + err.Error())
						return
					}
					if err := settings.Save(config); err != nil {
						a.backupResult("项目与标签页已导入，但设置保存失败：" + err.Error())
						return
					}
					a.update(func() {
						a.apply(config, false)
						a.ss.draft = map[string]string{}
						a.ss.backupBusy = false
						a.ss.toast = fmt.Sprintf("导入完成，新增 %d 个标签页", added)
						if a.setWin != nil {
							a.setWin.Invalidate()
						}
					})
				}()
			}}
			a.setWin.Invalidate()
		})
	}()
}

// configurationBackupGroup 提供只迁移元数据的本地导入导出入口。
func (a *App) configurationBackupGroup(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, "导入与导出", "保存应用设置、本机项目及标签页路径、分屏布局和 Node 版本，不包含终端内容、启动命令或历史记录。", func() {
		settingRow(c, pal, "配置文件", "导出为 JSON；设置包含环境变量和已保存的连接信息。导入合并项目与标签页，并应用设置。", func() {
			disabled := a.ss.backupBusy || a.local().cl == nil
			row := ui.Row(c).Gap(8).Disabled(disabled)
			if disabled {
				row.Opacity(0.5)
			}
			row.Children(func() {
				if button(c, "export-config", "导出配置…", "", false, pal) {
					a.exportConfiguration()
				}
				if button(c, "import-config", "导入配置…", "", true, pal) {
					a.importConfiguration()
				}
			})
		})
		if a.ss.backupBusy {
			ui.Text(c, "正在处理配置文件…").FontSize(12).TextColor(pal.textMuted)
		}
	})
}

// writeConfigurationBackup 完整写入后再替换目标，避免导出失败时截断已有备份。
func writeConfigurationBackup(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".panenest-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
