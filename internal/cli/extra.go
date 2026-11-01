package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"revy/internal/baseline"
	"revy/internal/collect"
	ghcollect "revy/internal/collect/github"
	"revy/internal/config"
	"revy/internal/facts"
	"revy/internal/policy"
)

// factsCmd 只采集事实不做策略判定（供智能体/外部工具消费）。
var factsCmd = &cobra.Command{
	Use:   "facts [path]",
	Short: "只采集 facts（JSON），不做策略判定",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot(cmd, args)
		cfg := config.Load(root)

		f := facts.Facts{
			SchemaVersion: facts.SchemaVersion,
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			Project:       facts.Project{Root: root},
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		gctx, gcancel := context.WithTimeout(ctx, 60*time.Second)
		defer gcancel()
		collect.Go(ctx, root, cfg, &f)
		collect.WebTS(ctx, root, cfg, &f)
		collect.Flutter(ctx, root, cfg, &f)
		ghcollect.Collect(gctx, root, cfg, &f)
		if fileExists(filepath.Join(root, "README.md")) {
			f.Docs.Readme = true
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(f)
	},
}

// policiesCmd 校验策略文件。
var policiesCmd = &cobra.Command{
	Use:   "policies [dir]",
	Short: "校验策略文件表达式可编译",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot(cmd, args)
		dir := filepath.Join(root, ".review", "policies")
		if len(args) > 0 {
			dir = args[0]
		}
		preset, err := policy.Load(mustPresetDir())
		if err != nil {
			return err
		}
		project, _ := policy.Load(dir)
		all := append(append([]policy.Policy{}, preset...), project...)
		errs := policy.Validate(all)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, "✘", e)
			}
			return fmt.Errorf("%d 条策略校验失败", len(errs))
		}
		fmt.Printf("✔ %d 条策略全部可编译（含预置包 %d 条）\n", len(all), len(preset))
		return nil
	},
}

// baselineCmd 基线管理。
var baselineCmd = &cobra.Command{
	Use:   "baseline",
	Short: "基线豁免管理",
}

var baselineUpdateCmd = &cobra.Command{
	Use:   "update [path]",
	Short: "把当前 fail 的策略 ID 写入基线（存量豁免，只管新增）",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot(cmd, args)
		cfg := config.Load(root)

		f := facts.Facts{SchemaVersion: facts.SchemaVersion, Project: facts.Project{Root: root}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		gctx, gcancel := context.WithTimeout(ctx, 60*time.Second)
		defer gcancel()
		collect.Go(ctx, root, cfg, &f)
		collect.WebTS(ctx, root, cfg, &f)
		collect.Flutter(ctx, root, cfg, &f)
		ghcollect.Collect(gctx, root, cfg, &f)

		preset, err := policy.Load(mustPresetDir())
		if err != nil {
			return err
		}
		project, _ := policy.Load(filepath.Join(root, ".review", "policies"))
		results := policy.Evaluate(append(append([]policy.Policy{}, preset...), project...), toMap(f))

		var failed []string
		for _, res := range results {
			if !res.Passed {
				failed = append(failed, res.Policy.ID)
			}
		}
		if err := baseline.Save(root, failed); err != nil {
			return err
		}
		if len(failed) == 0 {
			fmt.Println("✔ 当前无失败策略，基线已清空")
			return nil
		}
		fmt.Printf("✔ 已将 %d 条现存失败写入基线（只对新增违规按原严重度处理）：\n", len(failed))
		for _, id := range failed {
			fmt.Println("  -", id)
		}
		return nil
	},
}

// initCmd 初始化项目 .review 目录。
var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "初始化 .review/（配置 + 策略目录 + 空基线）",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot(cmd, args)
		dir := filepath.Join(root, ".review")
		for _, sub := range []string{"", "policies", "reports"} {
			if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "baseline.json"), []byte("{\"suppressed\": []}\n"), 0o644); err != nil {
			return err
		}
		cfgYaml := `# revy 项目配置
github:
  repo: ""            # owner/name；留空自动从 git remote 解析
go:
  env: {}             # 构建环境覆盖，如 {GOOS: linux}
checks:
  timeout_seconds: 180
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfgYaml), 0o644); err != nil {
			return err
		}
		example := `# 项目本地策略示例（与内置 strict 包合并执行，ID 相同时覆盖内置）
policies:
  - id: docs-context-exists
    dimension: docs
    severity: warn
    expr: "facts['docs']['readme']"
    message: "缺少 README"
`
		if err := os.WriteFile(filepath.Join(dir, "policies", "local.example.yaml"), []byte(example), 0o644); err != nil {
			return err
		}
		fmt.Println("✔ 已初始化", dir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(factsCmd, policiesCmd, baselineCmd, initCmd)
	baselineCmd.AddCommand(baselineUpdateCmd)
}
