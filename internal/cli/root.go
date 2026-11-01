// Package cli 定义 revy 的命令行入口。
package cli

import (
	"os"

	"github.com/spf13/cobra"
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

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2)
	}
}

func init() {
	rootCmd.PersistentFlags().StringP("format", "f", "terminal", "输出格式：terminal | json")
	rootCmd.PersistentFlags().StringP("project", "p", ".", "项目根目录")
	rootCmd.PersistentFlags().Bool("strict", false, "warn 升级为 fail")
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("revy {{.Version}}\n")

}
