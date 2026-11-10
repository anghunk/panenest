package daemon

import (
	"fmt"
	"os"
	"panenest/internal/settings"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// nodeBin 查找已安装版本，允许主版本、完整版本或明确的 bin 目录；不下载运行时。
func nodeBin(version string) (string, error) {
	version = strings.TrimSpace(version)
	if filepath.IsAbs(version) {
		if st, err := os.Stat(filepath.Join(version, "node")); err == nil && !st.IsDir() {
			return version, nil
		}
		return "", fmt.Errorf("Node bin 目录无效：%s", version)
	}
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) > 3 {
		return "", fmt.Errorf("Node 版本格式无效：%s", version)
	}
	for _, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 32); err != nil {
			return "", fmt.Errorf("Node 版本格式无效：%s", version)
		}
	}
	for _, installed := range installedNodes() {
		if installed.version == version || strings.HasPrefix(installed.version, version+".") {
			return installed.path, nil
		}
	}
	return "", fmt.Errorf("未找到已安装的 Node %s，请先通过 nvm / fnm 安装，或填写 Node bin 的绝对路径", version)
}

type nodeInstallation struct {
	path, version string
	nums          [3]uint64
}

// installedNodes 统一扫描可用于启动终端的已安装版本，按版本号降序排列。
func installedNodes() []nodeInstallation {
	home, _ := os.UserHomeDir()
	nvm := os.Getenv("NVM_DIR")
	if nvm == "" {
		nvm = filepath.Join(home, ".nvm")
	}
	fnm := os.Getenv("FNM_DIR")
	roots := []string{filepath.Join(nvm, "versions", "node"), filepath.Join(fnm, "node-versions"), filepath.Join(home, ".local", "share", "fnm", "node-versions"), filepath.Join(home, "Library", "Application Support", "fnm", "node-versions")}
	var found []nodeInstallation
	for _, root := range roots {
		if root == "node-versions" {
			continue
		}
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			v := strings.TrimPrefix(e.Name(), "v")
			bin := filepath.Join(root, e.Name(), "bin")
			if root != roots[0] {
				bin = filepath.Join(root, e.Name(), "installation", "bin")
			}
			if st, err := os.Stat(filepath.Join(bin, "node")); err != nil || st.IsDir() {
				continue
			}
			c := nodeInstallation{path: bin, version: v}
			parts := strings.Split(v, ".")
			valid := len(parts) == 3
			for i, p := range parts {
				if i >= 3 {
					break
				}
				n, err := strconv.ParseUint(p, 10, 32)
				if err != nil {
					valid = false
				}
				c.nums[i] = n
			}
			if !valid {
				continue
			}
			found = append(found, c)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		for k := 0; k < 3; k++ {
			if found[i].nums[k] != found[j].nums[k] {
				return found[i].nums[k] > found[j].nums[k]
			}
		}
		return found[i].path < found[j].path
	})
	return found
}

// nodeVersion 按标签页、项目、全局的顺序解析配置；调用方不持有 daemon 锁。
func (s *pane) nodeVersion() string {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	project := s.project
	if session := s.d.findSessionLocked(s.sid); session != nil {
		if session.nodeVersion != "" {
			return session.nodeVersion
		}
		project = session.project
	}
	for _, p := range s.d.projects {
		if p.ID == project && p.NodeVersion != "" {
			return p.NodeVersion
		}
	}
	return strings.TrimSpace(settings.Load().NodeVersion)
}

// nodeShellArgs 在用户启动文件加载后加入 Node 路径，避免登录 shell 覆盖选择。
func nodeShellArgs(shell, bin, node string, command []string) ([]string, []string, error) {
	name := shellName(shell)
	q := shellQuote(name, node)
	switch name {
	case "zsh":
		if err := writeZshenv(bin); err != nil {
			return nil, nil, err
		}
		args := shellArgs(shell, bin, command)
		if len(command) > 0 {
			args[len(args)-1] = "export PATH=" + q + `:"$PATH"; ` + args[len(args)-1]
		}
		return args, append(zshEnv(bin), "PANENEST_NODE_BIN="+node), nil
	case "fish":
		args := shellArgs(shell, bin, command)
		init := "set -gx PATH " + q + " $PATH"
		if len(command) > 0 {
			script := args[len(args)-1]
			if bin != "" {
				script = strings.ReplaceAll(script, shellQuote(name, fishBinFirst(bin)), shellQuote(name, fishBinFirst(bin)+"; "+init))
			}
			args[len(args)-1] = init + "; " + script
		}
		return append(args[:1], append([]string{"-C", init}, args[1:]...)...), nil, nil
	case "bash":
		dir, err := os.MkdirTemp("", "panenest-node-")
		if err != nil {
			return nil, nil, err
		}
		rc := filepath.Join(dir, "bashrc")
		script := `if [ -r ~/.bash_profile ]; then . ~/.bash_profile; elif [ -r ~/.bash_login ]; then . ~/.bash_login; elif [ -r ~/.profile ]; then . ~/.profile; fi
[ -r ~/.bashrc ] && . ~/.bashrc
export PATH=` + q + `:"$PATH"
rm -f ` + shellQuote(name, rc) + `; rmdir ` + shellQuote(name, dir) + "\n"
		if len(command) > 0 {
			script += shellJoin(name, command) + `; printf '\033]133;D;%s\007' "$?"` + "\n"
		}
		if err := os.WriteFile(rc, []byte(script), 0600); err != nil {
			return nil, nil, err
		}
		return []string{name, "--rcfile", rc, "-i"}, nil, nil
	}
	return nil, nil, fmt.Errorf("设置 Node 版本暂支持 zsh、bash 和 fish；当前 shell 为 %s", name)
}
