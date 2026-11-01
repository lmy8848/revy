package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Collector 采集 GitHub 侧事实：Actions 运行状态 + 三类安全告警。
type Collector struct {
	Repo  string // owner/name
	Token string
}

// ResolveToken 令牌解析顺序：环境变量 → git credential（要求在仓库目录内执行）。
func ResolveToken(repoDir string) string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	cmd := exec.Command("git", "credential", "fill")
	cmd.Dir = repoDir
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "password=") {
			return strings.TrimPrefix(line, "password=")
		}
	}
	return ""
}

// ParseRepoFromGit 从 git remote origin 解析 owner/name。
func ParseRepoFromGit(ctx context.Context, repoDir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", repoDir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	url = strings.TrimSuffix(url, ".git")
	for _, prefix := range []string{"https://github.com/", "git@github.com:"} {
		if strings.HasPrefix(url, prefix) {
			return strings.TrimPrefix(url, prefix)
		}
	}
	return ""
}

func (c *Collector) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("github api %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type ActionsRun struct {
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
}

// LatestRuns 返回默认分支最近 total 次工作流运行（含全部 workflow，
// 与面板 CI 页面口径一致）。
func (c *Collector) LatestRuns(ctx context.Context, branch string, total int) ([]ActionsRun, error) {
	var resp struct {
		WorkflowRuns []ActionsRun `json:"workflow_runs"`
	}
	path := fmt.Sprintf("/repos/%s/actions/runs?branch=%s&per_page=%d", c.Repo, branch, total)
	if err := c.get(ctx, path, &resp); err != nil {
		// 404：仓库无任何工作流运行，或令牌缺 Actions 读权限——按无数据
		if strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, err
	}
	return resp.WorkflowRuns, nil
}

type CodeScanningAlert struct {
	Number int `json:"number"`
	Rule   struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
	} `json:"rule"`
}

func (c *Collector) CodeScanningOpen(ctx context.Context) ([]CodeScanningAlert, error) {
	var alerts []CodeScanningAlert
	err := c.get(ctx, "/repos/"+c.Repo+"/code-scanning/alerts?state=open&per_page=100", &alerts)
	if err != nil && strings.Contains(err.Error(), "404") {
		return nil, nil // 未启用 code scanning 视为无告警
	}
	return alerts, err
}

type DependabotAlert struct {
	SecurityAdvisory struct {
		Severity string `json:"severity"`
	} `json:"security_advisory"`
}

func (c *Collector) DependabotOpen(ctx context.Context) ([]DependabotAlert, error) {
	var alerts []DependabotAlert
	err := c.get(ctx, "/repos/"+c.Repo+"/dependabot/alerts?state=open&per_page=100", &alerts)
	if err != nil && (strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "403")) {
		return nil, nil
	}
	return alerts, err
}

func (c *Collector) SecretScanningOpenCount(ctx context.Context) (int, error) {
	var alerts []map[string]any
	err := c.get(ctx, "/repos/"+c.Repo+"/secret-scanning/alerts?state=open&per_page=100", &alerts)
	if err != nil && (strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "403")) {
		return 0, nil
	}
	return len(alerts), err
}
