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

// nodeActivation 使用安装版本对应的管理器切换环境；自定义目录直接加入 PATH。
func nodeActivation(shell, node string) string {
	versionDir := filepath.Dir(node)
	if shell != "fish" && filepath.Base(filepath.Dir(versionDir)) == "node" && filepath.Base(filepath.Dir(filepath.Dir(versionDir))) == "versions" {
		root := filepath.Dir(filepath.Dir(filepath.Dir(versionDir)))
		version := filepath.Base(versionDir)
		return "if ! command -v nvm >/dev/null 2>&1; then . " + shellQuote(shell, filepath.Join(root, "nvm.sh")) + "; fi; nvm use --silent " + shellQuote(shell, version)
	}
	if filepath.Base(versionDir) == "installation" {
		version := filepath.Base(filepath.Dir(versionDir))
		if shell == "fish" {
			return "if command -q fnm; fnm use " + shellQuote(shell, version) + "; else; set -gx PATH " + shellQuote(shell, node) + " $PATH; end"
		}
		return `if command -v fnm >/dev/null 2>&1; then eval "$(fnm env --shell ` + shellQuote(shell, shell) + `)"; fnm use ` + shellQuote(shell, version) + "; else export PATH=" + shellQuote(shell, node) + `:"$PATH"; fi`
	}
	if shell == "fish" {
		return "set -gx PATH " + shellQuote(shell, node) + " $PATH"
	}
	return "export PATH=" + shellQuote(shell, node) + `:"$PATH"`
}

// nodeShellArgs 在用户启动文件加载后切换 Node，避免登录 shell 覆盖选择。
func nodeShellArgs(shell, bin, node string, command []string, control ...string) ([]string, []string, error) {
	name := shellName(shell)
	activation := ""
	if node != "" {
		activation = nodeActivation(name, node)
	}
	initial := activation
	if len(control) > 0 && control[0] != "" {
		activation = nodeLiveSetup(name, control[0], activation)
	}
	switch name {
	case "zsh":
		if err := writeZshenv(bin); err != nil {
			return nil, nil, err
		}
		args := shellArgs(shell, bin, command)
		if len(command) > 0 {
			if initial != "" {
				args[len(args)-1] = initial + "; " + args[len(args)-1]
			}
		}
		return args, append(zshEnv(bin), "PANENEST_NODE_INIT="+activation), nil
	case "fish":
		args := shellArgs(shell, bin, command)
		init := activation
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
` + activation + `
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

// nodeLiveSetup 注册当前 shell 的切换回调；信号只发送给已确认安装回调的进程。
func nodeLiveSetup(name, control, initial string) string {
	q := shellQuote(name, control)
	applying := shellQuote(name, control+".applying")
	ready := shellQuote(name, control+".ready")
	if initial != "" {
		initial += "; "
	}
	if name == "fish" {
		return "set -g __panenest_node_original_path $PATH; function __panenest_node_update --on-signal WINCH; if test -f " + q + "; command mv " + q + " " + applying + "; source " + applying + " >/dev/null; command rm -f " + applying + "; end; end; " + initial + "printf '%s' $fish_pid > " + ready
	}
	return `__panenest_node_original_path="$PATH"; ` + "__panenest_node_update() { if [ -f " + q + " ]; then command mv " + q + " " + applying + " && { . " + applying + " >/dev/null; command rm -f " + applying + "; }; fi; }; trap '__panenest_node_update' WINCH; " + initial + `printf '%s' "$$" > ` + ready
}

// nodeControlPath 是本终端专用的切换脚本，不修改用户的 shell 配置。
func (s *pane) nodeControlPath() string { return filepath.Join(s.d.dir, "node", s.info.ID+".sh") }

// syncNodeVersion 在 shell 空闲时静默切换版本，保留编辑中的输入；忙碌时下次重试。
func (s *pane) syncNodeVersion() {
	version := s.nodeVersion()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.proc == nil || s.info.Browser() || s.nodeVersionApplied == version || s.nodeControl == "" {
		return
	}
	if s.proc.Foreground() != s.shellPID {
		return
	}
	ready, err := os.ReadFile(s.nodeControl + ".ready")
	if err != nil || string(ready) != strconv.Itoa(s.shellPID) {
		return
	}
	name := s.info.Shell
	var activation string
	if version == "" {
		if name == "fish" {
			activation = "if functions -q nvm; nvm use default; else if command -q fnm; fnm use default; else; set -gx PATH $__panenest_node_original_path; end"
		} else {
			activation = "if command -v nvm >/dev/null 2>&1; then nvm use --silent default; elif command -v fnm >/dev/null 2>&1; then fnm use default; else export PATH=\"$__panenest_node_original_path\"; fi"
		}
	} else {
		node, err := nodeBin(version)
		if err != nil {
			return
		}
		activation = nodeActivation(name, node)
	}
	if err := writeAtomic(s.nodeControl, []byte(activation+"\n")); err != nil {
		return
	}
	if err := s.proc.notifyNodeVersion(); err != nil {
		return
	}
	s.nodeVersionApplied = version
}
