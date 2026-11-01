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

var runCmd = &cobra.Command{
	Use:   "run [path]",
	Short: "采集事实 → 策略判定 → 输出报告（默认命令）",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReview,
}

var runOutDir string

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
}

func runReview(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	strict, _ := cmd.Flags().GetBool("strict")
	root := projectRoot(cmd, args)

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
		return err
	}
	projectPolicies, err := policy.Load(filepath.Join(root, ".review", "policies"))
	if err != nil {
		return err
	}
	all := append(append([]policy.Policy{}, preset...), projectPolicies...)

	results := policy.Evaluate(all, toMap(f))
	base := baseline.Load(root).Suppress()

	pass, warn, fail := 0, 0, 0
	var baselineUsed []string
	for i := range results {
		res := &results[i]
		sev := res.Policy.Severity
		suppressed := false
		if !res.Passed {
			if _, sup := base[res.Policy.ID]; sup {
				sev, suppressed = policy.SevWarn, true
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
		res.Passed = res.Passed || (sev == policy.SevWarn && !strings.Contains(res.Error, ""))
		if strict && sev == policy.SevWarn && !res.Passed {
			fail++
			warn--
			res.Policy.Severity = policy.SevError
		}
		_ = suppressed
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

	// ── 输出 ──
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(runResult{
			Facts: f, Results: results, Passed: run.Passed,
			Fail: fail, Warn: warn, Pass: pass,
			DurationMS: run.DurationMS, StartedAt: run.StartedAt.Format(time.RFC3339),
			ProjectRoot: root, BaselineUsed: baselineUsed,
		})
	default:
		fmt.Print(report.Terminal(&run))
		if html, err := report.HTML(&run); err == nil {
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

	if fail > 0 {
		return errExitFail
	}
	return nil
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

func mustPresetDir() string {
	// 预置包以 embed 提供，写进临时目录复用 Load 的目录语义
	dir, err := os.MkdirTemp("", "revy-preset-*")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "strict.yaml"), []byte(policy.PresetStrict()), 0o644); err != nil {
		panic(err)
	}
	return dir
}

// errExitFail 存在 fail（退出码 1）；运行错误由 cobra 返回 error（退出码 2）。
var errExitFail = fmt.Errorf("存在未通过的策略")

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil && !os.IsNotExist(err)
}

// projectRoot 解析项目根：位置参数优先，其次 -p flag，缺省当前目录。
func projectRoot(cmd *cobra.Command, args []string) string {
	if len(args) > 0 {
		abs, _ := filepath.Abs(args[0])
		return abs
	}
	p, _ := cmd.Flags().GetString("project")
	abs, _ := filepath.Abs(p)
	return abs
}
