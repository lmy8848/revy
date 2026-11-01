package github

import (
	"context"

	"revy/internal/config"
	"revy/internal/facts"
)

// Collect 把 GitHub 侧事实写入 facts（token/仓库不可解析时 found=false）。
func Collect(ctx context.Context, root string, cfg *config.Config, f *facts.Facts) {
	repo := cfg.GitHub.Repo
	if repo == "" {
		repo = ParseRepoFromGit(ctx, root)
	}
	if repo == "" {
		f.GitHub.SkipReason = "无法从 git remote 或配置解析仓库"
		return
	}
	token := ResolveToken(root)
	if token == "" {
		f.GitHub.SkipReason = "未找到 GitHub 凭据（GITHUB_TOKEN 或 git credential）"
		return
	}
	c := &Collector{Repo: repo, Token: token}

	// 仓库信息拿默认分支
	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	branch := "master"
	if err := c.get(ctx, "/repos/"+repo, &repoInfo); err == nil && repoInfo.DefaultBranch != "" {
		branch = repoInfo.DefaultBranch
	}

	f.GitHub.Found = true
	f.GitHub.Repo = repo

	runs, err := c.LatestRuns(ctx, branch, 5)
	if err == nil {
		f.GitHub.Actions.RecentTotal = len(runs)
		success := 0
		for i, r := range runs {
			if i == 0 {
				f.GitHub.Actions.Latest = facts.LatestRun{
					Conclusion: r.Conclusion, HeadSHA: r.HeadSHA, Name: r.Name,
				}
			}
			if r.Conclusion == "success" {
				success++
			}
		}
		f.GitHub.Actions.RecentSuccessCount = success
		if len(runs) > 0 {
			f.GitHub.Actions.RecentSuccessRatio = float64(success) / float64(len(runs))
		}
	}

	if cs, err := c.CodeScanningOpen(ctx); err == nil {
		for _, a := range cs {
			f.GitHub.CodeScanning.OpenTotal++
			switch a.Rule.Severity {
			case "critical":
				f.GitHub.CodeScanning.OpenCritical++
			case "error", "high":
				f.GitHub.CodeScanning.OpenHigh++
			case "warning", "medium":
				f.GitHub.CodeScanning.OpenModerate++
			}
		}
	}

	if db, err := c.DependabotOpen(ctx); err == nil {
		for _, a := range db {
			f.GitHub.Dependabot.OpenTotal++
			switch a.SecurityAdvisory.Severity {
			case "critical":
				f.GitHub.Dependabot.OpenCritical++
			case "high":
				f.GitHub.Dependabot.OpenHigh++
			case "medium", "moderate":
				f.GitHub.Dependabot.OpenModerate++
			}
		}
	}

	if n, err := c.SecretScanningOpenCount(ctx); err == nil {
		f.GitHub.SecretScanning.OpenTotal = n
	}
}
