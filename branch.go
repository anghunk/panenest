package main

import (
	"strings"

	"panenest/internal/proto"

	"panenest/locales"
)

// branchName is a worktree's branch, or its commit while detached.
func branchName(g *proto.GitInfo) string {
	if g.Branch != "" {
		return g.Branch
	}
	if len(g.Head) >= 7 {
		return g.Head[:7]
	}
	return g.Head
}

// gitState names an operation in progress.
func gitState(st string) string {
	if say, ok := map[string]func() string{
		"merge": locales.GitMerging, "rebase": locales.GitRebasing, "cherry-pick": locales.GitCherryPicking, "revert": locales.GitReverting,
	}[st]; ok {
		return say()
	}
	return ""
}

// gitSummary says a worktree's status in a line: its branch, how far it
// is from its upstream, and what changed, as "fix-login ↑2 · 3 files +40 −2".
func gitSummary(g *proto.GitInfo) string {
	var b strings.Builder
	b.WriteString(branchName(g))
	if g.Ahead > 0 {
		b.WriteString(" ↑" + itoa(g.Ahead))
	}
	if g.Behind > 0 {
		b.WriteString(" ↓" + itoa(g.Behind))
	}
	if st := gitState(g.State); st != "" {
		b.WriteString(" · " + st)
	}
	if g.Files > 0 {
		b.WriteString(" · " + locales.ChangesFiles(g.Files))
		if g.Added > 0 {
			b.WriteString(" +" + itoa(g.Added))
		}
		if g.Removed > 0 {
			b.WriteString(" −" + itoa(g.Removed))
		}
	}
	return b.String()
}

// sessionGit 返回会话悬停提示使用的 Git 信息：优先取主导分屏，其次取焦点分屏或其他分屏。
func sessionGit(s *proto.Session) *proto.GitInfo {
	if s.Git != nil {
		return s.Git
	}
	if p := s.FocusPane(); p != nil && p.Git != nil {
		return p.Git
	}
	for i := range s.Panes {
		if s.Panes[i].Git != nil {
			return s.Panes[i].Git
		}
	}
	return nil
}
