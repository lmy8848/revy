package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"revy/internal/baseline"
	"revy/internal/collect"
	ghcollect "revy/internal/collect/github"
	"revy/internal/config"
	"revy/internal/facts"
	"revy/internal/policy"
	"revy/internal/report"
)

var runOutDir string

var runCmd = &cobra.Command{
	Use:   "run [path]",
	Short: "采集事实 → 策略判定 → 输出报告（默认命令）",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReview,
}

func init() {
	runCmd.Flags().StringVar(&runOutDir, "out", "", "HTML 报告输出目录（缺省写到 <root>/.review/reports/）")
	rootCmd.AddCommand(runCmd)
}

type runResult struct {
	Facts        facts.Facts     `json:"facts"`
	Results      []policy.Result `json:"policies"`
	Passed       bool            `json:"passed"`
	Fail         int             `json:"fail"`
	Warn         int             `json:"warn"`
	Pass         int             `json:"pass"`
	DurationMS   int64           `json:"duration_ms"`
	StartedAt    string          `json:"started_at"`
	ProjectRoot  string          `json:"project_root"`
	BaselineUsed []string        `json:"baseline_used,omitempty"`
	Trend        report.Trend    `json:"trend,omitempty"`
	ArtifactPath string          `json:"artifact_path,omitempty"`
}

func runReview(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	strict, _ := cmd.Flags().GetBool("strict")
	root := projectRoot(cmd, args)

	artifact, err := RunReviewFor(root, strict)
	if err != nil {
		return err
	}

	run := artifactToRun(artifact)
	switch format {
	case "json":
		_ = json.NewEncoder(os.Stdout).Encode(artifact)
	default:
		fmt.Print(report.Terminal(&run))
		fmt.Printf("  工件: %s\n", artifact.ArtifactPath)
		if html, err := report.HTMLWithTrend(&run, artifact.Trend.New, artifact.Trend.Resolved); err == nil {
			outDir := runOutDir
			if outDir == "" {
				outDir = filepath.Join(root, ".review", "reports")
			}
			_ = os.MkdirAll(outDir, 0o755)
			htmlPath := filepath.Join(outDir, "report-"+time.Now().Format("20060102-150405")+".html")
			if werr := os.WriteFile(htmlPath, []byte(html), 0o644); werr == nil {
				fmt.Printf("  HTML 报告: %s\n", htmlPath)
			}
		}
	}

	if artifact.Fail > 0 {
		return errExitFail
	}
	return nil
}

func artifactToRun(a runResult) report.Run {
	return report.Run{
		StartedAt:    mustTime(a.StartedAt),
		DurationMS:   a.DurationMS,
		ProjectRoot:  a.ProjectRoot,
		Facts:        a.Facts,
		Results:      a.Results,
		BaselineUsed: suppressSet(a.BaselineUsed),
		Passed:       a.Passed,
	}
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}

// RunReviewFor 对指定项目执行完整审查并落工件（serve/桌面端/CLI 共用）。
func RunReviewFor(root string, strict bool) (runResult, error) {
	start := time.Now()
	cfg := config.Load(root)

	// ── 采集 facts ──
	f := facts.Facts{
		SchemaVersion: facts.SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Project:       facts.Project{Root: root},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	goCtx, gcancel := context.WithTimeout(ctx, 60*time.Second)
	defer gcancel()
	collect.Go(ctx, root, cfg, &f)
	collect.WebTS(ctx, root, cfg, &f)
	collect.Flutter(ctx, root, cfg, &f)
	ghcollect.Collect(goCtx, root, cfg, &f)

	if fileExists(filepath.Join(root, "README.md")) || fileExists(filepath.Join(root, "README_zh.md")) {
		f.Docs.Readme = true
	}

	// ── 策略加载与求值 ──
	preset, err := policy.Load(mustPresetDir())
	if err != nil {
		return runResult{}, err
	}
	projectPolicies, err := policy.Load(filepath.Join(root, ".review", "policies"))
	if err != nil {
		return runResult{}, err
	}
	all := append(append([]policy.Policy{}, preset...), projectPolicies...)

	results := policy.Evaluate(all, toMap(f))
	base := baseline.Load(root).Suppress()

	pass, warn, fail := 0, 0, 0
	var baselineUsed []string
	for i := range results {
		res := &results[i]
		sev := res.Policy.Severity
		if !res.Passed {
			if _, sup := base[res.Policy.ID]; sup {
				sev = policy.SevWarn
				baselineUsed = append(baselineUsed, res.Policy.ID)
			}
		}
		switch {
		case res.Passed:
			pass++
		case sev == policy.SevError:
			fail++
		default:
			warn++
		}
		if strict && sev == policy.SevWarn && !res.Passed {
			fail++
			warn--
			res.Policy.Severity = policy.SevError
		}
	}

	run := report.Run{
		StartedAt:    start,
		DurationMS:   time.Since(start).Milliseconds(),
		ProjectRoot:  root,
		Facts:        f,
		Results:      results,
		BaselineUsed: suppressSet(baselineUsed),
		Passed:       fail == 0,
	}

	// ── 工件落盘 + 趋势对比 ──
	runsDir := filepath.Join(root, ".review", "runs")
	_ = os.MkdirAll(runsDir, 0o755)
	prev, _ := loadPrevFailing(runsDir, start)

	artifact := runResult{
		Facts: f, Results: results, Passed: run.Passed,
		Fail: fail, Warn: warn, Pass: pass,
		DurationMS: run.DurationMS, StartedAt: run.StartedAt.Format(time.RFC3339),
		ProjectRoot: root, BaselineUsed: baselineUsed,
	}
	artifact.Trend = trendFrom(prev, artifact)
	artifact.ArtifactPath = filepath.Join(runsDir, "run-"+start.Format("20060102-150405")+".json")
	if data, aerr := json.MarshalIndent(artifact, "", "  "); aerr == nil {
		_ = os.WriteFile(artifact.ArtifactPath, data, 0o644)
	}

	if artifact.Fail > 0 {
		return artifact, errExitFail
	}
	return artifact, nil
}

// trendFrom 对比上次工件的失败集合：新增 / 已解决。
func trendFrom(prev prevFailingInfo, artifact runResult) report.Trend {
	t := report.Trend{New: []string{}, Resolved: []string{}}
	curFailing := map[string]struct{}{}
	for _, res := range artifact.Results {
		if !res.Passed {
			curFailing[res.Policy.ID] = struct{}{}
		}
	}
	for _, id := range prev.failing {
		if _, still := curFailing[id]; !still {
			t.Resolved = append(t.Resolved, id)
		}
	}
	for id := range curFailing {
		if _, was := prev.failingSet[id]; !was {
			t.New = append(t.New, id)
		}
	}
	return t
}

// prevFailingInfo 上一次运行工件的失败策略信息。
type prevFailingInfo struct {
	failing    []string
	failingSet map[string]struct{}
}

// loadPrevFailing 找开始时间之前最近的一次运行工件，取其失败策略集合。
func loadPrevFailing(runsDir string, before time.Time) (prevFailingInfo, string) {
	empty := prevFailingInfo{failing: []string{}, failingSet: map[string]struct{}{}}
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return empty, ""
	}
	var latest string
	var latestT time.Time
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "run-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ts, err := time.Parse("20060102-150405", strings.TrimSuffix(strings.TrimPrefix(e.Name(), "run-"), ".json"))
		if err != nil || !ts.Before(before) {
			continue
		}
		if latest == "" || ts.After(latestT) {
			latest, latestT = e.Name(), ts
		}
	}
	if latest == "" {
		return empty, ""
	}
	data, err := os.ReadFile(filepath.Join(runsDir, latest))
	if err != nil {
		return empty, latest
	}
	var art runResult
	if err := json.Unmarshal(data, &art); err != nil {
		return empty, latest
	}
	info := prevFailingInfo{failing: []string{}, failingSet: map[string]struct{}{}}
	for _, res := range art.Results {
		if !res.Passed {
			info.failing = append(info.failing, res.Policy.ID)
			info.failingSet[res.Policy.ID] = struct{}{}
		}
	}
	return info, latest
}

// toMap 把 facts 转为 CEL 可消费的 map（经 JSON 往返抹平类型）。
func toMap(f facts.Facts) map[string]any {
	data, _ := json.Marshal(f)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

func suppressSet(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// errExitFail 存在 fail（退出码 1）；运行错误由 cobra 返回 error（退出码 2）。
var errExitFail = fmt.Errorf("存在未通过的策略")

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil && !os.IsNotExist(err)
}
