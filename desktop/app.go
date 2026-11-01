package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"revy/internal/review"
)

// App 是桌面端暴露给前端（Wails 绑定）的方法集合。
type App struct {
	ctx context.Context
}

func NewApp() *App { return &App{} }

// Startup 保存运行时上下文（事件发射用）。
func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

// RunReview 执行完整审查并返回结果（同步，可能耗时数分钟）。
func (a *App) RunReview(root string, strict bool) (review.ReviewResult, error) {
	return review.Run(root, review.Options{Strict: strict})
}

// ListRuns 列出项目的历史审查工件。
func (a *App) ListRuns(root string) []string {
	runsDir := filepath.Join(root, ".review", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return []string{}
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// GetArtifact 读取工件内容（JSON 字符串）。
func (a *App) GetArtifact(root, file string) string {
	file = filepath.Base(file) // 防目录穿越
	data, err := os.ReadFile(filepath.Join(root, ".review", "runs", file))
	if err != nil {
		return ""
	}
	return string(data)
}

// ReadReport 读取指定 HTML 报告内容（前端内嵌展示）。
func (a *App) ReadReport(root, file string) string {
	file = filepath.Base(file)
	data, err := os.ReadFile(filepath.Join(root, ".review", "reports", file))
	if err != nil {
		return ""
	}
	return string(data)
}
