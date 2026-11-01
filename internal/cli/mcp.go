package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"revy/internal/collect"
	"revy/internal/config"
	"revy/internal/facts"
)

// mcpCmd 以 stdio 运行 MCP 服务器：智能体（ZCode/Claude/Cursor 等）可把
// revy 作为 MCP 工具原生接入，无需解析 CLI 文本输出。
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "以 MCP 服务器（stdio）运行，供智能体原生调用",
	Long: `启动 revy 的 MCP 服务器。在客户端配置：
  命令: revy
  参数: ["mcp"]
暴露工具：run_review / get_facts / list_runs / get_artifact`,
	RunE: func(cmd *cobra.Command, args []string) error {
		server := mcp.NewServer(&mcp.Implementation{Name: "revy", Version: version}, nil)

		mcp.AddTool(server, &mcp.Tool{
			Name:        "run_review",
			Description: "对指定项目执行完整审查（事实采集 + CEL 策略判定），返回通过/告警/失败统计、失败策略明细与工件路径。同步执行，可能耗时数分钟。",
		}, mcpReview)

		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_facts",
			Description: "只采集项目事实（facts JSON：lint/build/test/CI/安全告警/依赖图/路由表），不做策略判定。",
		}, mcpFacts)

		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_runs",
			Description: "列出项目 .review/runs/ 下的历史审查工件。",
		}, mcpRuns)

		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_artifact",
			Description: "读取指定审查工件的完整内容（facts + 策略结果 + 趋势）。",
		}, mcpArtifact)

		fmt.Fprintln(os.Stderr, "revy MCP 服务器已启动（stdio）")
		return server.Run(context.Background(), &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

// ── 工具输入/输出结构（json tag 即 MCP schema）──

type mcpReviewIn struct {
	Root   string `json:"root" jsonschema:"项目根目录的绝对路径"`
	Strict bool   `json:"strict,omitempty" jsonschema:"warn 升级为 fail"`
}

type mcpPathIn struct {
	Root string `json:"root" jsonschema:"项目根目录的绝对路径"`
}

type mcpArtifactIn struct {
	Root string `json:"root" jsonschema:"项目根目录的绝对路径"`
	File string `json:"file" jsonschema:"工件文件名，如 run-20261101-120000.json"`
}

// loadCfg 读取项目配置。
func loadCfg(root string) *config.Config {
	return config.Load(root)
}

// timeNowUTC 当前 UTC 时间（RFC3339）。
func timeNowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// mcpReview 运行审查。策略失败不是错误——结果里带 fail 统计与明细。
func mcpReview(ctx context.Context, req *mcp.CallToolRequest, in mcpReviewIn) (*mcp.CallToolResult, any, error) {
	if in.Root == "" {
		return nil, nil, fmt.Errorf("root 不能为空")
	}
	artifact, err := RunReviewFor(in.Root, in.Strict)
	if err != nil && artifact.Pass == 0 && artifact.Fail == 0 {
		return nil, nil, err // 真运行错误
	}
	var failing []string
	for _, r := range artifact.Results {
		if !r.Passed {
			failing = append(failing, r.Policy.ID+": "+firstNonEmpty(r.Error, r.Policy.Message))
		}
	}
	out := map[string]any{
		"passed":       artifact.Passed,
		"pass":         artifact.Pass,
		"warn":         artifact.Warn,
		"fail":         artifact.Fail,
		"project_root": artifact.ProjectRoot,
		"failing":      failing,
		"trend":        artifact.Trend,
		"artifact":     artifact.ArtifactPath,
		"report":       artifact.ReportPath,
	}
	return nil, out, nil
}

// mcpFacts 采集 facts。
func mcpFacts(ctx context.Context, req *mcp.CallToolRequest, in mcpPathIn) (*mcp.CallToolResult, any, error) {
	if in.Root == "" {
		return nil, nil, fmt.Errorf("root 不能为空")
	}
	cfg := loadCfg(in.Root)
	f := facts.Facts{
		SchemaVersion: facts.SchemaVersion,
		GeneratedAt:   timeNowUTC(),
		Project:       facts.Project{Root: in.Root},
	}
	gctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	collect.Go(gctx, in.Root, cfg, &f)
	collect.WebTS(gctx, in.Root, cfg, &f)
	collect.Flutter(gctx, in.Root, cfg, &f)
	return nil, f, nil
}

// mcpRuns 列出历史工件。
func mcpRuns(ctx context.Context, req *mcp.CallToolRequest, in mcpPathIn) (*mcp.CallToolResult, any, error) {
	runsDir := filepath.Join(in.Root, ".review", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return nil, []string{}, nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	return nil, out, nil
}

// mcpArtifact 读取工件内容。
func mcpArtifact(ctx context.Context, req *mcp.CallToolRequest, in mcpArtifactIn) (*mcp.CallToolResult, any, error) {
	if in.Root == "" || in.File == "" {
		return nil, nil, fmt.Errorf("root 与 file 不能为空")
	}
	// 防目录穿越：只取文件名
	file := filepath.Base(in.File)
	data, err := os.ReadFile(filepath.Join(in.Root, ".review", "runs", file))
	if err != nil {
		return nil, nil, fmt.Errorf("工件不存在: %w", err)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, nil, err
	}
	return nil, artifact, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
