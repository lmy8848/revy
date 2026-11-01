package collect

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SSHExecutor 把检查派发到 Linux 远端执行：项目经 tar 流同步到远端
// 工作目录，检查结果经 SSH 流回。适用于 Windows 宿主审查 Linux 项目
// 且需要真实执行（非交叉编译）的场景。
type SSHExecutor struct {
	Host string // user@host
	Key  string // 私钥路径；空则用默认凭据
	Dir  string // 远端工作根目录
}

func (e *SSHExecutor) keyArgs() []string {
	if e.Key == "" {
		return nil
	}
	return []string{"-i", e.Key, "-o", "BatchMode=yes"}
}

// Sync 把项目目录同步到远端（排除 .git/.review/build 等本地工件），
// 使用 tar 流避免逐文件 scp。远端目录 = Dir/<项目名>。
func (e *SSHExecutor) Sync(ctx context.Context, root string) (string, error) {
	projectName := filepath.Base(root)
	remote := e.Dir + "/" + projectName

	// 清理远端旧内容再接收（保持幂等）
	if err := runSilent(e.sshArgs(ctx, "rm -rf "+remote)); err != nil {
		return "", err
	}
	if err := runSilent(e.sshArgs(ctx, "mkdir -p "+remote)); err != nil {
		return "", err
	}

	tar := exec.CommandContext(ctx, "tar", "-C", root,
		"--exclude=.git", "--exclude=.review", "--exclude=node_modules",
		"--exclude=build", "--exclude=dist",
		"-cf", "-", ".")
	extract := e.sshArgs(ctx, "tar -C "+remote+" -xf -")
	archive, _ := tar.StdoutPipe()
	extract.Stdin = archive
	if err := tar.Start(); err != nil {
		return "", err
	}
	if err := extract.Start(); err != nil {
		_ = tar.Process.Kill()
		return "", err
	}
	if err := extract.Wait(); err != nil {
		return "", err
	}
	_ = tar.Wait()
	return remote, nil
}

// Run 在远端执行命令并返回合并输出。
func (e *SSHExecutor) Run(ctx context.Context, remoteDir, command string, timeout time.Duration) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(e.keyArgs(),
		"-o", "StrictHostKeyChecking=accept-new",
		e.Host,
		"cd "+remoteDir+" && "+command)
	out, err := exec.CommandContext(cctx, "ssh", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (e *SSHExecutor) sshArgs(ctx context.Context, remoteCmd string) *exec.Cmd {
	args := append([]string{"-o", "StrictHostKeyChecking=accept-new"}, e.keyArgs()...)
	args = append(args, e.Host, remoteCmd)
	return exec.CommandContext(ctx, "ssh", args...)
}

func runSilent(cmd *exec.Cmd) error {
	_, err := cmd.CombinedOutput()
	return err
}
