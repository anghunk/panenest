package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"panenest/internal/proto"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// A command a session starts with runs in a process group of its own, in
// the terminal's foreground, so that tick sees it; on Windows, as the
// shell's child.
func TestCommandInForeground(t *testing.T) {
	shells, command, want := []string{"fish", "zsh", "bash"}, []string{"sleep", "5"}, "sleep"
	if runtime.GOOS == "windows" {
		shells, command, want = []string{"pwsh", "powershell", "cmd"}, []string{"ping", "-n", "6", "127.0.0.1"}, "ping"
	}
	for _, name := range shells {
		t.Run(name, func(t *testing.T) {
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skip(name, "not installed")
			}
			proc, err := startPTY(ptyOptions{
				Path: shell, Args: shellArgs(shell, "", command),
				Dir: t.TempDir(), Env: testEnv(t),
				Cols: 80, Rows: 24,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer proc.Kill()
			go func() {
				buf := make([]byte, 4096)
				for {
					if _, err := proc.Read(buf); err != nil {
						return
					}
				}
			}()
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
				if pg := proc.Foreground(); pg != 0 && pg != proc.PID() && strings.EqualFold(procName(pg), want) {
					return
				}
			}
			t.Fatalf("foreground %d is not %s's (shell %d)", proc.Foreground(), want, proc.PID())
		})
	}
}

// A shell tells when the command a session starts with ends, with its
// exit status, and goes on.
func TestCommandTellsExit(t *testing.T) {
	shells, command := []string{"fish", "zsh", "bash", "dash"}, []string{"sh", "-c", "exit 3"}
	if runtime.GOOS == "windows" {
		shells, command = []string{"pwsh", "powershell"}, []string{"cmd", "/c", "exit 3"}
	}
	for _, name := range shells {
		t.Run(name, func(t *testing.T) {
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skip(name, "not installed")
			}
			if !marksExit(shell) {
				t.Fatal("marksExit is false")
			}
			proc, err := startPTY(ptyOptions{
				Path: shell, Args: shellArgs(shell, "", command),
				Dir: t.TempDir(), Env: testEnv(t),
				Cols: 80, Rows: 24,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer proc.Kill()
			got := make(chan string, 16)
			go func() {
				var o oscScanner
				buf := make([]byte, 4096)
				for {
					n, err := proc.Read(buf)
					o.scan(buf[:n], func(b []byte) { got <- string(b) }, func() {})
					if err != nil {
						return
					}
				}
			}()
			for timeout := time.After(10 * time.Second); ; {
				select {
				case b := <-got:
					if strings.HasPrefix(b, "133;D") {
						if b != "133;D;3" {
							t.Fatalf("got %q, want 133;D;3", b)
						}
						return
					}
				case <-timeout:
					t.Fatal("no OSC 133;D")
				}
			}
		})
	}
}

func testEnv(t *testing.T) []string {
	if runtime.GOOS != "windows" {
		return []string{"HOME=" + t.TempDir(), "TERM=xterm-256color"}
	}
	// Windows' programs need its own variables to start.
	return append(exec.Command("cmd").Environ(), "TERM=xterm-256color")
}

// TestInstalledNodeVersions 验证列表扫描与启动解析使用同一套版本、数值排序和过滤规则。
func TestInstalledNodeVersions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	nvm, fnm := filepath.Join(home, "nvm"), filepath.Join(home, "fnm")
	t.Setenv("NVM_DIR", nvm)
	t.Setenv("FNM_DIR", fnm)
	for _, rel := range []string{"nvm/versions/node/v22.9.0/bin", "nvm/versions/node/v22.10.0/bin", "fnm/node-versions/v20.18.0/installation/bin", "nvm/versions/node/invalid/bin"} {
		dir := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "node"), nil, 0755); err != nil {
			t.Fatal(err)
		}
	}
	var versions []string
	for _, n := range installedNodes() {
		versions = append(versions, n.version)
	}
	if !slices.Equal(versions, []string{"22.10.0", "22.9.0", "20.18.0"}) {
		t.Fatalf("versions: %v", versions)
	}
	bin, err := nodeBin("22")
	if err != nil || bin != filepath.Join(nvm, "versions", "node", "v22.10.0", "bin") {
		t.Fatalf("resolved bin %q: %v", bin, err)
	}
	if _, err := nodeBin("24"); err == nil {
		t.Fatal("missing version resolved")
	}
}

// TestNodeVersionShellStartup 验证 nvm 启动与实时切换，保留输入且不向终端写入切换命令。
func TestNodeVersionShellStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 POSIX shell")
	}
	for _, name := range []string{"zsh", "bash"} {
		t.Run(name, func(t *testing.T) {
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skip("未安装 " + name)
			}
			for _, withCommand := range []bool{false, true} {
				t.Run(fmt.Sprint(withCommand), func(t *testing.T) {
					home := t.TempDir()
					selected := filepath.Join(home, "nvm", "versions", "node", "v99.1.2", "bin")
					fallback := filepath.Join(home, "nvm", "versions", "node", "v18.0.0", "bin")
					t.Setenv("HOME", home)
					t.Setenv("NVM_DIR", filepath.Join(home, "nvm"))
					t.Setenv("FNM_DIR", filepath.Join(home, "fnm"))
					for dir, version := range map[string]string{selected: "v99.1.2", fallback: "v18.0.0"} {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\necho "+version+"\n"), 0755); err != nil {
							t.Fatal(err)
						}
					}
					marker := filepath.Join(home, "nvm-calls")
					nvm := "nvm() { printf '%s\\n' \"$*\" >> " + shellQuote("zsh", marker) + `; export PATH="$NVM_DIR/versions/node/$3/bin:$PATH"; }` + "\n"
					if err := os.WriteFile(filepath.Join(home, "nvm", "nvm.sh"), []byte(nvm), 0600); err != nil {
						t.Fatal(err)
					}
					rc := "export PATH=" + shellQuote("zsh", fallback) + `:"$PATH"` + "\nPS1='PANENEST_READY> '\n"
					if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0600); err != nil {
						t.Fatal(err)
					}
					var command []string
					if withCommand {
						command = []string{"node", "--version"}
					}
					bin := filepath.Join(home, "app", "bin")
					control := filepath.Join(home, "live-node.sh")
					args, extra, err := nodeShellArgs(shell, bin, selected, command, control)
					if err != nil {
						t.Fatal(err)
					}
					env := append(testEnv(t), "HOME="+home, "NVM_DIR="+filepath.Join(home, "nvm"))
					env = append(env, extra...)
					proc, err := startPTY(ptyOptions{Path: shell, Args: args, Dir: home, Env: env, Cols: 100, Rows: 30})
					if err != nil {
						t.Fatal(err)
					}
					defer proc.Kill()
					chunks := make(chan string, 32)
					go func() {
						buf := make([]byte, 4096)
						for {
							n, err := proc.Read(buf)
							if n > 0 {
								chunks <- string(buf[:n])
							}
							if err != nil {
								close(chunks)
								return
							}
						}
					}()
					timer := time.NewTimer(5 * time.Second)
					defer timer.Stop()
					output := ""
					sent := false
					for {
						select {
						case chunk, ok := <-chunks:
							if !ok {
								t.Fatalf("shell ended: %s", output)
							}
							output += chunk
							if !sent && strings.Contains(output, "PANENEST_READY>") {
								sent = true
								proc.Write([]byte("node --version\n"))
							}
							want := 1
							if withCommand {
								want = 2
							}
							if sent && strings.Count(output, "v99.1.2") >= want {
								calls, err := os.ReadFile(marker)
								if err != nil || !strings.Contains(string(calls), "use --silent v99.1.2") {
									t.Fatalf("未调用 nvm use: %s, %v", calls, err)
								}
								// 切换时保留当前输入，并确认没有把切换命令写入终端。
								d := projectDaemon(t)
								live := &pane{d: d, sid: "live", proc: proc, shellPID: proc.PID(), nodeControl: control, nodeVersionApplied: "99.1.2", info: proto.Pane{ID: "live-pane", Shell: name}}
								d.sessions = []*session{{id: "live", nodeVersion: "18.0.0"}}
								proc.Write([]byte("sleep 0.2\n"))
								busyDeadline := time.Now().Add(time.Second)
								for {
									pg := proc.Foreground()
									if pg != proc.PID() && procName(pg) == "sleep" {
										break
									}
									if time.Now().After(busyDeadline) {
										t.Fatal("测试程序未进入前台")
									}
									time.Sleep(5 * time.Millisecond)
								}
								live.syncNodeVersion()
								if live.nodeVersionApplied != "99.1.2" {
									t.Fatal("在前台程序运行中切换了 Node")
								}
								for proc.Foreground() != proc.PID() {
									if time.Now().After(busyDeadline) {
										t.Fatal("测试程序未结束")
									}
									time.Sleep(5 * time.Millisecond)
								}
								proc.Write([]byte("printf 'LIVE_DRAFT\\n'"))
								live.syncNodeVersion()
								deadline := time.Now().Add(2 * time.Second)
								for {
									live.syncNodeVersion()
									calls, _ := os.ReadFile(marker)
									if strings.Contains(string(calls), "use --silent v18.0.0") {
										break
									}
									if time.Now().After(deadline) {
										t.Fatal("shell 未在提示符中处理实时切换")
									}
									time.Sleep(10 * time.Millisecond)
								}
								proc.Write([]byte("\nnode --version\n"))
								for {
									select {
									case chunk, ok := <-chunks:
										if !ok {
											t.Fatal("shell ended during switch")
										}
										output += chunk
										// 新版 Bash 在执行命令前输出括号粘贴控制序列；按可见文本检查，避免误判草稿丢失。
										visible := strings.NewReplacer("\x1b[?2004h", "", "\x1b[?2004l", "", "\r", "").Replace(output)
										if strings.Contains(visible, "\nLIVE_DRAFT\n") && strings.Contains(visible, "\nv18.0.0\n") {
											if strings.Contains(output, "export PATH=") || strings.Contains(output, "nvm use --silent") {
												t.Fatalf("切换命令出现在终端：%s", output)
											}
											return
										}
									case <-timer.C:
										t.Fatalf("实时切换后版本或输入不正确：%s", output)
									}
								}
							}
						case <-timer.C:
							t.Fatalf("所选 Node 未生效: %s", output)
						}
					}
				})
			}
		})
	}
}
