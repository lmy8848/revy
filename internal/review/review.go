// Package review 是 revy 的核心编排引擎：采集 facts → 策略判定 →
// 基线豁免 → 工件落盘。CLI/serve/MCP/桌面端都是它的视图适配器。
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revy/internal/baseline"
	"revy/internal/collect"
	ghcollect "revy/internal/collect/github"
	"revy/internal/config"
	"revy/internal/facts"
	"revy/internal/policy"
	"revy/internal/report"
)

// ReviewResult 一次审查运行的完整结果（JSON 工件的同构体）。
type ReviewResult struct {
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
	Trend        Trend           `json:"trend,omitempty"`
	ArtifactPath string          `json:"artifact_path,omitempty"`
	ReportPath   string          `json:"report_path,omitempty"`
}

// Trend 趋势对比（与上次运行）：新增 / 已解决的失败策略。
type Trend struct {
	New      []string `json:"new,omitempty"`
	Resolved []string `json:"resolved,omitempty"`
}

// ProgressEvent 审查过程的实时进度事件（SSE/桌面端推送用）。
type ProgressEvent struct {
	Stage   string `json:"stage"` // collect / policy
	Name    string `json:"name"`
	State   string `json:"state"` // running / ok / fail
	Message string `json:"message,omitempty"`
}

// Options 审查运行选项。
type Options struct {
	Strict  bool
	OnEvent func(ProgressEvent) // nil 安全；并行采集时可能从多个 goroutine 调用
}

// Run 对指定项目执行完整审查并落工件。
func Run(root string, opts Options) (ReviewResult, error) {
	return run(context.Background(), root, opts)
}

// RunWithContext 同 Run，允许调用方控制取消（如 SSE 断连）。
func RunWithContext(ctx context.Context, root string, opts Options) (ReviewResult, error) {
	return run(ctx, root, opts)
}

func run(ctx context.Context, root string, opts Options) (ReviewResult, error) {
	start := time.Now()
	cfg := config.Load(root)

	emit := func(stage, name, state, msg string) {
		if opts.OnEvent != nil {
			opts.OnEvent(ProgressEvent{Stage: stage, Name: name, State: state, Message: msg})
		}
	}

	// ── 采集 facts ──
	f := facts.Facts{
		SchemaVersion: facts.SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Project:       facts.Project{Root: root},
	}
	goCtx, gcancel := context.WithTimeout(ctx, 120*time.Second)
	defer gcancel()

	emit("collect", "Go 检查（build/vet/test/lint）", "running", "")
	collect.Go(ctx, root, cfg, &f)
	emit("collect", "Go 检查（build/vet/test/lint）", "ok", "")

	emit("collect", "Web 检查（typecheck/lint/test）", "running", "")
	collect.WebTS(ctx, root, cfg, &f)
	emit("collect", "Web 检查（typecheck/lint/test）", "ok", "")

	emit("collect", "Flutter 检查（analyze/test）", "running", "")
	collect.Flutter(ctx, root, cfg, &f)
	emit("collect", "Flutter 检查（analyze/test）", "ok", "")

	emit("collect", "GitHub 采集（CI/安全告警）", "running", "")
	ghcollect.Collect(goCtx, root, cfg, &f)
	emit("collect", "GitHub 采集（CI/安全告警）", "ok", "")

	if fileExists(filepath.Join(root, "README.md")) || fileExists(filepath.Join(root, "README_zh.md")) {
		f.Docs.Readme = true
	}

	// ── 策略加载与求值 ──
	emit("policy", "策略判定", "running", "")
	preset, err := policy.LoadPreset()
	if err != nil {
		return ReviewResult{}, err
	}
	projectPolicies, err := policy.Load(filepath.Join(root, ".review", "policies"))
	if err != nil {
		return ReviewResult{}, err
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
		if opts.Strict && sev == policy.SevWarn && !res.Passed {
			fail++
			warn--
			res.Policy.Severity = policy.SevError
		}
	}
	emit("policy", "策略判定", "ok", fmt.Sprintf("%d pass · %d warn · %d fail", pass, warn, fail))

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

	artifact := ReviewResult{
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

	// HTML 报告（CLI/serve/桌面端统一在此生成）
	reportsDir := filepath.Join(root, ".review", "reports")
	_ = os.MkdirAll(reportsDir, 0o755)
	if html, herr := report.HTMLWithTrend(&run, artifact.Trend.New, artifact.Trend.Resolved); herr == nil {
		artifact.ReportPath = filepath.Join(reportsDir, "report-"+start.Format("20060102-150405")+".html")
		if werr := os.WriteFile(artifact.ReportPath, []byte(html), 0o644); werr != nil {
			artifact.ReportPath = ""
		}
	}

	return artifact, nil
}

// trendFrom 对比上次工件的失败集合：新增 / 已解决。
func trendFrom(prev prevFailingInfo, artifact ReviewResult) Trend {
	t := Trend{New: []string{}, Resolved: []string{}}
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
	var art ReviewResult
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil && !os.IsNotExist(err)
}
