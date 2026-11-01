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
	cmd.Env = append(cmd.Environ(), r.Env...)
	outBytes, err := cmd.CombinedOutput()
	durMS = time.Since(start).Milliseconds()

	out := strings.TrimSpace(string(outBytes))
	if len(out) > 4096 {
		out = out[len(out)-4096:]
	}
	if cctx.Err() == context.DeadlineExceeded {
		return false, false, "超时（" + r.Timeout.String() + "）", durMS
	}
	return err == nil, false, out, durMS
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

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}
