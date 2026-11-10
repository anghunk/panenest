package daemon

import (
	"errors"
	"fmt"
	"panenest/internal/proto"
	"slices"
	"time"
)

// exportWorkspace 在锁内生成独立的元数据快照，不复制终端内容和启动命令。
func (d *Daemon) exportWorkspace() proto.WorkspaceRecords {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := proto.WorkspaceRecords{Projects: slices.Clone(d.projects)}
	for _, s := range d.sessions {
		tab := proto.WorkspaceTab{Project: s.project, Name: s.name, NodeVersion: s.nodeVersion, Layout: s.layout.Clone()}
		for _, p := range s.panes {
			p.mu.Lock()
			tab.Panes = append(tab.Panes, proto.WorkspacePane{ID: p.info.ID, Dir: p.info.Dir, URL: p.info.URL})
			p.mu.Unlock()
		}
		out.Tabs = append(out.Tabs, tab)
	}
	return out
}

// validateWorkspace 拒绝无效关联和布局；导入失败不会更改已有项目和标签页。
func validateWorkspace(in proto.WorkspaceRecords) error {
	if len(in.Projects) > 1000 || len(in.Tabs) > 5000 {
		return errors.New("工作区记录过多")
	}
	projects := map[string]bool{"": true}
	for _, p := range in.Projects {
		if p.ID == "" || projects[p.ID] {
			return errors.New("项目标识重复或为空")
		}
		projects[p.ID] = true
	}
	for _, tab := range in.Tabs {
		if !projects[tab.Project] {
			return errors.New("标签页关联的项目不存在")
		}
		if len(tab.Panes) == 0 || len(tab.Panes) > 64 {
			return errors.New("标签页的分屏数量无效")
		}
		ids := map[string]bool{}
		for _, p := range tab.Panes {
			if p.ID == "" || ids[p.ID] {
				return errors.New("分屏标识重复或为空")
			}
			ids[p.ID] = true
		}
		var walk func(*proto.Layout, int) bool
		walk = func(l *proto.Layout, depth int) bool {
			if l == nil || depth > 64 {
				return false
			}
			if l.Leaf() {
				if !ids[l.Pane] {
					return false
				}
				delete(ids, l.Pane)
				return true
			}
			return l.Pane == "" && l.Ratio > 0 && l.Ratio < 1 && walk(l.A, depth+1) && walk(l.B, depth+1)
		}
		if tab.Layout == nil {
			if len(tab.Panes) != 1 {
				return errors.New("分屏布局缺失")
			}
		} else if !walk(tab.Layout, 0) || len(ids) != 0 {
			return errors.New("分屏布局无效")
		}
	}
	return nil
}

// importWorkspace 合并项目和标签页；新终端保持停止状态，不自动运行任何命令。
func (d *Daemon) importWorkspace(in proto.WorkspaceRecords) (int, error) {
	if err := validateWorkspace(in); err != nil {
		return 0, err
	}
	var staged []*session
	cleanup := func() {
		for _, s := range staged {
			for _, p := range s.panes {
				if p.term != nil {
					p.term.Close()
				}
			}
		}
	}
	now := time.Now().UnixMilli()
	for _, tab := range in.Tabs {
		s := &session{id: newID(), name: tab.Name, project: tab.Project, nodeVersion: tab.NodeVersion, created: now}
		staged = append(staged, s)
		ids := map[string]string{}
		for _, sp := range tab.Panes {
			id := newID()
			ids[sp.ID] = id
			p, err := newPane(d, s.id, proto.Pane{ID: id, Dir: sp.Dir, URL: sp.URL, Status: proto.Exited, Created: now, Since: now, Cols: 120, Rows: 36})
			if err != nil {
				cleanup()
				return 0, fmt.Errorf("创建导入标签页：%w", err)
			}
			s.panes = append(s.panes, p)
		}
		s.layout = tab.Layout.Clone()
		var remap func(*proto.Layout)
		remap = func(l *proto.Layout) {
			if l == nil {
				return
			}
			if l.Leaf() {
				l.Pane = ids[l.Pane]
				return
			}
			remap(l.A)
			remap(l.B)
		}
		remap(s.layout)
		s.focus = s.panes[0].info.ID
	}
	d.mu.Lock()
	mapping := map[string]string{"": ""}
	for _, pr := range in.Projects {
		for _, existing := range d.projects {
			if pr.Dir != "" && existing.Dir == pr.Dir || pr.Dir == "" && existing.Dir == "" && existing.Name == pr.Name {
				mapping[pr.ID] = existing.ID
				break
			}
		}
		if mapping[pr.ID] == "" {
			old := pr.ID
			pr.ID = newID()
			mapping[old] = pr.ID
			d.projects = append(d.projects, pr)
		}
	}
	if d.panes == nil {
		d.panes = map[string]*pane{}
	}
	added := 0
	for _, s := range staged {
		s.project = mapping[s.project]
		duplicate := false
		for _, existing := range d.sessions {
			if existing.project != s.project || existing.name != s.name || existing.nodeVersion != s.nodeVersion || len(existing.panes) != len(s.panes) {
				continue
			}
			same := true
			for i, p := range existing.panes {
				p.mu.Lock()
				same = same && p.info.Dir == s.panes[i].info.Dir && p.info.URL == s.panes[i].info.URL
				p.mu.Unlock()
			}
			if same {
				duplicate = true
				break
			}
		}
		if duplicate {
			for _, p := range s.panes {
				if p.term != nil {
					p.term.Close()
				}
			}
			continue
		}
		d.sessions = append(d.sessions, s)
		added++
		for _, p := range s.panes {
			d.panes[p.info.ID] = p
		}
	}
	d.mu.Unlock()
	d.changed()
	return added, nil
}
