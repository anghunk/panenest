package proto

// WorkspaceRecords 仅保存本机工作区结构，不包含进程、命令、历史或终端内容。
type WorkspaceRecords struct {
	Projects []Project      `json:"projects"`
	Tabs     []WorkspaceTab `json:"tabs"`
}

// WorkspaceTab 保存标签页的名称、归属、路径和分屏布局。
type WorkspaceTab struct {
	Project     string          `json:"project"`
	Name        string          `json:"name,omitempty"`
	NodeVersion string          `json:"nodeVersion,omitempty"`
	Panes       []WorkspacePane `json:"panes"`
	Layout      *Layout         `json:"layout,omitempty"`
}

// WorkspacePane 的 ID 仅用于关联导出文件中的布局，导入时重新分配。
type WorkspacePane struct {
	ID  string `json:"id"`
	Dir string `json:"dir,omitempty"`
	URL string `json:"url,omitempty"`
}
