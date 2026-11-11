package main

import (
	"crypto/rand"
	"slices"
	"strings"
)

// projectFolder 是项目列表中的单层逻辑文件夹，不对应磁盘目录。
type projectFolder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

// projectFolderByID 返回指定文件夹；空标识表示未分组，不对应文件夹。
func (a *App) projectFolderByID(id string) *projectFolder {
	if id == "" {
		return nil
	}
	for i := range a.prefs.ProjectFolders {
		if a.prefs.ProjectFolders[i].ID == id {
			return &a.prefs.ProjectFolders[i]
		}
	}
	return nil
}

// projectFolderOf 返回项目所属的有效文件夹，兼容空项目标识和旧偏好。
func (a *App) projectFolderOf(key string) string {
	id := a.prefs.ProjectFolder[key]
	if a.projectFolderByID(id) == nil {
		return ""
	}
	return id
}

// createProjectFolder 创建展开的文件夹并保存；空名称不创建文件夹。
func (a *App) createProjectFolder(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	id := rand.Text()
	a.prefs.ProjectFolders = append(a.prefs.ProjectFolders, projectFolder{ID: id, Name: name})
	a.savePrefs()
	return id
}

// renameProjectFolder 更新文件夹名称，忽略空名称及已删除的文件夹。
func (a *App) renameProjectFolder(id, name string) {
	folder := a.projectFolderByID(id)
	name = strings.TrimSpace(name)
	if folder == nil || name == "" || folder.Name == name {
		return
	}
	folder.Name = name
	a.savePrefs()
}

// deleteProjectFolder 删除文件夹并将其项目移回未分组，不关闭项目或会话。
func (a *App) deleteProjectFolder(id string) {
	i := slices.IndexFunc(a.prefs.ProjectFolders, func(folder projectFolder) bool { return folder.ID == id })
	if i < 0 {
		return
	}
	a.prefs.ProjectFolders = slices.Delete(a.prefs.ProjectFolders, i, i+1)
	for key, folder := range a.prefs.ProjectFolder {
		if folder == id {
			delete(a.prefs.ProjectFolder, key)
		}
	}
	a.savePrefs()
}

// moveProjectFolder 将文件夹移到目标之前或之后，不改变各文件夹内的项目顺序。
func (a *App) moveProjectFolder(id, target string, after bool) {
	from := slices.IndexFunc(a.prefs.ProjectFolders, func(folder projectFolder) bool { return folder.ID == id })
	to := slices.IndexFunc(a.prefs.ProjectFolders, func(folder projectFolder) bool { return folder.ID == target })
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
	folder := a.prefs.ProjectFolders[from]
	a.prefs.ProjectFolders = slices.Delete(a.prefs.ProjectFolders, from, from+1)
	a.prefs.ProjectFolders = slices.Insert(a.prefs.ProjectFolders, to, folder)
	a.savePrefs()
}

// setProjectFolder 仅更新项目归属，由调用者与排序变更一起保存。
func (a *App) setProjectFolder(key, folder string) {
	if folder == "" {
		delete(a.prefs.ProjectFolder, key)
		return
	}
	if a.prefs.ProjectFolder == nil {
		a.prefs.ProjectFolder = map[string]string{}
	}
	a.prefs.ProjectFolder[key] = folder
}

// moveProjectToFolder 将已加载项目追加到目标分组末尾；空文件夹表示未分组。
// 尚未加载的远程项目保留原有归属和相对顺序。
func (a *App) moveProjectToFolder(key, folder string) {
	if folder != "" && a.projectFolderByID(folder) == nil {
		return
	}
	order := slices.Clone(a.prefs.ProjectOrder)
	found := false
	for _, g := range a.groups() {
		project := g.key()
		found = found || project == key
		if !slices.Contains(order, project) {
			order = append(order, project)
		}
	}
	if !found {
		return
	}
	from := slices.Index(order, key)
	order = slices.Delete(order, from, from+1)
	to := len(order)
	for i, project := range order {
		if a.projectFolderOf(project) == folder {
			to = i + 1
		}
	}
	order = slices.Insert(order, to, key)
	if a.projectFolderOf(key) == folder && slices.Equal(order, a.prefs.ProjectOrder) {
		return
	}
	a.setProjectFolder(key, folder)
	a.prefs.ProjectOrder = order
	a.savePrefs()
}
