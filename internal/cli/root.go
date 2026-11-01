// Package cli 定义 revy 的命令行入口。
package cli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"revy/internal/policy"
	"strings"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "revy",
	Short: "策略即代码的项目体检台",
	Long: `revy —— 采集项目事实（facts），以策略脚本（CEL）判定，输出
终端摘要 / JSON / 单文件 HTML 报告。面向智能体：全部结果
--format json 可结构化消费，退出码 0=通过 1=存在 fail 2=运行错误。`,
	SilenceUsage: true,
}

// ErrExitFail 存在未通过的策略（退出码 1）。
var ErrExitFail = errors.New("存在未通过的策略")

// Execute 执行根命令。返回 ErrExitFail 表示策略失败（退出码 1），
// 其它错误为运行错误（退出码 2）。
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		if strings.Contains(err.Error(), ErrExitFail.Error()) {
			return ErrExitFail
		}
		return err
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringP("format", "f", "terminal", "输出格式：terminal | json")
	rootCmd.PersistentFlags().StringP("project", "p", ".", "项目根目录")
	rootCmd.PersistentFlags().Bool("strict", false, "warn 升级为 fail")
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("revy {{.Version}}\n")

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

// mustPresetDir 预置策略包以 embed 提供，写进临时目录复用目录加载语义。
func mustPresetDir() string {
	dir, err := os.MkdirTemp("", "revy-preset-*")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "strict.yaml"), []byte(policy.PresetStrict()), 0o644); err != nil {
		panic(err)
	}
	return dir
}
