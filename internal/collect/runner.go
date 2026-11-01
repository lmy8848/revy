// Package collect 实现 facts 采集器：每个适配器把一类检查结果写入 facts。
package collect

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// runCheck 执行一条命令并产出 CheckResult：超时/失败不中断采集，
// 失败输出保留尾部 4KB 供报告与策略使用。
type runCheck struct {
	Name    string
	Cmd     string
	Args    []string
	Dir     string
	Env     []string
	Timeout time.Duration
}

func (r runCheck) run(ctx context.Context) (ok, skipped bool, output string, durMS int64) {
	if _, err := exec.LookPath(r.Cmd); err != nil {
		return false, true, r.Cmd + " 不在 PATH 中，检查跳过", 0
	}
	cctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(cctx, r.Cmd, r.Args...)
	cmd.Dir = r.Dir
	cmd.Env = windowsPathFix(append(cmd.Environ(), r.Env...))
	outBytes, err := cmd.CombinedOutput()
	durMS = time.Since(start).Milliseconds()

	out := strings.TrimSpace(string(outBytes))
	if len(out) > 4096 {
		out = out[len(out)-4096:]
	}
	if cctx.Err() == context.DeadlineExceeded {
		return false, false, "超时（" + r.Timeout.String() + "）", durMS
	}
	// 子进程环境损坏（PATH 缺系统目录/批处理宿主异常）：不是被审查项目的
	// 缺陷，标记跳过并写明原因，避免把宿主环境问题算到项目头上
	if envBroken(out) {
		return false, true, "执行环境异常（子进程 PATH 损坏或缺少系统目录）——请在干净的终端运行", durMS
	}
	return err == nil, false, out, durMS
}

// envBroken 识别 Windows 批处理环境损坏的典型输出（中英文）。
func envBroken(out string) bool {
	markers := []string{
		"不是内部或外部命令", "is not recognized as an internal",
		"Unable to find git in your PATH",
	}
	for _, m := range markers {
		if strings.Contains(out, m) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// execCommand 便捷执行：返回合并输出（已 TrimSpace）。
func execCommand(ctx context.Context, cmd string, dir string, args ...string) (string, error) {
	c, err := exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	_ = dir
	return strings.TrimSpace(string(c)), err
}

func lookPathOK(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func goos() string { return runtime.GOOS }

// windowsPathFix Windows 下补系统目录到子进程 PATH：
// .bat 脚本（flutter.bat 等）内部依赖 where/git 等系统命令，
// 而从 Git Bash 等环境继承的 PATH 可能缺失 System32，导致批处理
// 内部命令全部失效（表现为 WHERE/git 不是内部或外部命令）。
func windowsPathFix(env []string) []string {
	if goos() != "windows" {
		return env
	}
	const sysDirs = `C:\Windows\System32;C:\Windows`
	for i, e := range env {
		if strings.HasPrefix(strings.ToLower(e), "path=") {
			if !strings.Contains(strings.ToLower(e), "system32") {
				env[i] = e + ";" + sysDirs
			}
			return env
		}
	}
	return append(env, "PATH="+sysDirs)
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}
