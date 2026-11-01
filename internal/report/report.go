// Package report 把一次审查运行渲染为终端 / JSON / HTML 三种视图。
// 三视图同源于 Run 结构，互不掺私货。
package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"strings"
	"time"

	"revy/internal/facts"
	"revy/internal/policy"
)

//go:embed mermaid.min.js
var mermaidJS string

// Run 一次审查运行的完整结果。
type Run struct {
	StartedAt    time.Time
	DurationMS   int64
	ProjectRoot  string
	Facts        any // facts.Facts
	Results      []policy.Result
	BaselineUsed map[string]struct{}
	Passed       bool // 全部通过（warn 不算失败）
}

// EffectiveSeverity 计算生效严重度：被基线豁免的失败降级为 warn。
func (r *Run) EffectiveSeverity(res policy.Result) (policy.Severity, bool) {
	if res.Passed {
		return res.Policy.Severity, false
	}
	if _, suppressed := r.BaselineUsed[res.Policy.ID]; suppressed {
		return policy.SevWarn, true
	}
	return res.Policy.Severity, false
}

// FailedIDs 返回按生效严重度属于 fail 的策略 ID（供基线写入）。
func (r *Run) FailedIDs() []string {
	var ids []string
	for _, res := range r.Results {
		sev, _ := r.EffectiveSeverity(res)
		if !res.Passed && sev == policy.SevError {
			ids = append(ids, res.Policy.ID)
		}
	}
	return ids
}

// Terminal 渲染彩色终端摘要。
func Terminal(r *Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nrevy 审查报告 · %s · %s\n", r.ProjectRoot, r.StartedAt.Format("2006-01-02 15:04:05"))
	b.WriteString(strings.Repeat("─", 62) + "\n")

	pass, warn, fail := 0, 0, 0
	lastDim := ""
	for _, res := range r.Results {
		sev, suppressed := r.EffectiveSeverity(res)
		dimChanged := res.Policy.Dimension != lastDim
		lastDim = res.Policy.Dimension

		switch {
		case res.Passed:
			pass++
			b.WriteString(green(fmt.Sprintf("  ✔ %-12s %s\n", res.Policy.Dimension, res.Policy.ID)))
		case sev == policy.SevError:
			fail++
			b.WriteString(red(fmt.Sprintf("  ✘ %-12s %s%s\n", res.Policy.Dimension, res.Policy.ID, suppressMark(suppressed))))
		default:
			warn++
			b.WriteString(yellow(fmt.Sprintf("  ▲ %-12s %s%s\n", res.Policy.Dimension, res.Policy.ID, suppressMark(suppressed))))
		}
		if !res.Passed {
			msg := res.Policy.Message
			if res.Error != "" {
				msg = "表达式错误: " + res.Error
			}
			b.WriteString(grey(fmt.Sprintf("      └─ %s\n", msg)))
		}
		if dimChanged {
			b.WriteString(grey("      ─\n"))
		}
	}
	b.WriteString(strings.Repeat("─", 62) + "\n")
	verdict := green("通过")
	if fail > 0 {
		verdict = red(fmt.Sprintf("未通过（%d fail）", fail))
	} else if warn > 0 {
		verdict = yellow(fmt.Sprintf("通过（%d warn）", warn))
	}
	fmt.Fprintf(&b, "  %d pass · %d warn · %d fail  →  %s\n", pass, warn, fail, verdict)
	return b.String()
}

func suppressMark(suppressed bool) string {
	if suppressed {
		return "（基线豁免）"
	}
	return ""
}

func green(s string) string  { return "\033[32m" + s + "\033[0m" }
func red(s string) string    { return "\033[31m" + s + "\033[0m" }
func yellow(s string) string { return "\033[33m" + s + "\033[0m" }
func grey(s string) string   { return "\033[90m" + s + "\033[0m" }

// ── HTML 视图：Go 侧预构建视图模型，模板零逻辑 ──

type htmlRow struct {
	Mark, SevClass, ID, Msg, Tag, TagClass string
}

type htmlDim struct {
	Name string
	Rows []htmlRow
}

type htmlRoute struct{ Method, Path string }
type htmlRouteGroup struct {
	Domain string
	Routes []htmlRoute
}
type htmlTrend struct{ New, Resolved []string }

type htmlView struct {
	Root, Meta, Verdict, VerdictClass string
	Fail, Warn, Pass                  int
	Dims                              []htmlDim
	Mermaid                           template.HTML
	MermaidJS                         template.HTML
	Routes                            []htmlRouteGroup
	Trend                             *htmlTrend
}

// ViewModel 预构建 HTML 渲染视图。
func ViewModel(r *Run) htmlView {
	return viewModel(r, nil, nil)
}

// ViewModelWithTrend 预构建带趋势对比的视图。
func ViewModelWithTrend(r *Run, newFailures, resolved []string) htmlView {
	return viewModel(r, newFailures, resolved)
}

func viewModel(r *Run, newFailures, resolved []string) htmlView {
	v := htmlView{
		Root: r.ProjectRoot,
		Meta: fmt.Sprintf("%s · %s · 耗时 %dms", r.ProjectRoot, r.StartedAt.Format("2006-01-02 15:04:05"), r.DurationMS),
		Dims: []htmlDim{},
	}
	// 架构图（Mermaid）+ 路由表
	if gf, ok := r.Facts.(facts.Facts); ok {
		if mg := MermaidGraph(gf); mg != "" {
			v.Mermaid = template.HTML(mg)
			v.MermaidJS = template.HTML(mermaidJS)
		}
		last := ""
		for _, rt := range gf.Go.Routes {
			if rt.Domain != last {
				v.Routes = append(v.Routes, htmlRouteGroup{Domain: rt.Domain})
				last = rt.Domain
			}
			g := &v.Routes[len(v.Routes)-1]
			g.Routes = append(g.Routes, htmlRoute{Method: rt.Method, Path: rt.Path})
		}
	}
	if newFailures != nil || resolved != nil {
		v.Trend = &htmlTrend{New: newFailures, Resolved: resolved}
	}
	dimIndex := map[string]int{}
	for _, res := range r.Results {
		sev, suppressed := r.EffectiveSeverity(res)
		row := htmlRow{ID: res.Policy.ID}
		switch {
		case res.Passed:
			row.Mark, row.SevClass = "✔", "ok"
			v.Pass++
		case sev == policy.SevError:
			row.Mark, row.SevClass = "✘", "fail"
			v.Fail++
			row.Tag, row.TagClass = "error", "err"
			if suppressed {
				row.Tag, row.TagClass = "error · 基线豁免", "err"
			}
		default:
			row.Mark, row.SevClass = "▲", "warn"
			v.Warn++
			row.Tag, row.TagClass = "warn", "warn"
			if suppressed {
				row.Tag, row.TagClass = "warn · 基线豁免", "warn"
			}
		}
		row.Msg = res.Policy.Message
		if res.Error != "" {
			row.Msg = "表达式错误: " + res.Error
		}
		idx, seen := dimIndex[res.Policy.Dimension]
		if !seen {
			v.Dims = append(v.Dims, htmlDim{Name: res.Policy.Dimension})
			idx = len(v.Dims) - 1
			dimIndex[res.Policy.Dimension] = idx
		}
		v.Dims[idx].Rows = append(v.Dims[idx].Rows, row)
	}
	switch {
	case v.Fail > 0:
		v.Verdict, v.VerdictClass = fmt.Sprintf("未通过 · %d fail", v.Fail), "fail"
	case v.Warn > 0:
		v.Verdict, v.VerdictClass = fmt.Sprintf("通过 · %d warn", v.Warn), "pass"
	default:
		v.Verdict, v.VerdictClass = "通过", "pass"
	}
	return v
}

// Trend 趋势对比（与上次运行）。
type Trend struct {
	New      []string // 本次新增失败
	Resolved []string // 上次失败本次已解决
}

// HTML 渲染单文件报告（无趋势）。
func HTML(r *Run) (string, error) {
	return HTMLWithTrend(r, nil, nil)
}

// HTMLWithTrend 渲染带趋势对比的单文件报告。
func HTMLWithTrend(r *Run, newFailures, resolved []string) (string, error) {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, ViewModelWithTrend(r, newFailures, resolved)); err != nil {
		return "", err
	}
	return b.String(), nil
}

//go:embed template.html
var htmlTemplate string
