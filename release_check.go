package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"panenest/internal/update"
)

const releaseAPI = "https://api.github.com/repos/anghunk/panenest/releases/latest"

// releaseInfo 是用于版本提示的正式发布信息，不参与自动安装。
type releaseInfo struct {
	Tag        string `json:"tag_name"`
	Body       string `json:"body"`
	URL        string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// updatePrefs 保留手动检查状态；关闭设置时取消请求并使旧结果失效。
type updatePrefs struct {
	checking    bool
	available   bool
	comparable  bool
	lastChecked time.Time
	release     releaseInfo
	err         error
	cancel      context.CancelFunc
	generation  uint64
}

// latestRelease 在 ctx 的期限内读取大小受限的 GitHub 响应，并验证版本及下载页面。
func latestRelease(ctx context.Context) (releaseInfo, error) {
	var release releaseInfo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return release, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PaneNest")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release, fmt.Errorf("GitHub: %s", resp.Status)
	}
	const maxResponse = 1 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return release, err
	}
	if len(data) > maxResponse {
		return release, fmt.Errorf("GitHub response exceeds 1 MiB")
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return release, err
	}
	u, err := url.Parse(release.URL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil ||
		!strings.HasPrefix(u.Path, "/anghunk/panenest/releases/tag/") ||
		!update.ValidVersion(strings.TrimPrefix(release.Tag, "v")) || release.Draft || release.Prerelease {
		return releaseInfo{}, fmt.Errorf("Invalid GitHub release metadata")
	}
	return release, nil
}

// checkReleaseUpdate 在后台检查版本；结果只写回仍有效的设置窗口。
func (a *App) checkReleaseUpdate() {
	u := &a.ss.updates
	if u.checking {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	u.cancel = cancel
	u.generation++
	generation := u.generation
	u.checking = true
	u.err = nil
	current := strings.TrimPrefix(mygo.App.Version(), "v")
	go func() {
		defer cancel()
		release, err := latestRelease(ctx)
		a.update(func() {
			u := &a.ss.updates
			if u.generation != generation || a.setWin == nil {
				return
			}
			u.checking = false
			u.cancel = nil
			u.err = err
			u.available = false
			if err == nil {
				u.lastChecked = time.Now()
				u.release = release
				u.comparable = update.ValidVersion(current)
				u.available = u.comparable && update.Compare(release.Tag, current) > 0
			}
			a.setWin.Invalidate()
		})
	}()
}
