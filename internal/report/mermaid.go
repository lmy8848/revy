package report

import (
	"fmt"
	"sort"
	"strings"

	"revy/internal/facts"
)

// MermaidGraph 把 import 依赖图渲染成 Mermaid graph TD。
// 节点按 internal/ 第一段分层（domain/infra/api/feature...），
// 同层多子包合并为一个节点显示（保持图可读）。
func MermaidGraph(f facts.Facts) string {
	nodes := map[string]string{} // segment -> layer
	edges := map[string][]string{}

	layerOf := func(seg string) string {
		parts := strings.SplitN(seg, "/", 2)
		return parts[0]
	}

	for _, e := range f.Go.ImportGraph.Edges {
		for _, seg := range []string{e.From, e.To} {
			if _, ok := nodes[seg]; !ok {
				nodes[seg] = layerOf(seg)
			}
		}
		if e.From == e.To {
			continue
		}
		edges[e.From] = append(edges[e.From], e.To)
	}
	if len(nodes) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("graph TD\n")
	// 按层分组子图，层内节点紧凑
	byLayer := map[string][]string{}
	var layers []string
	for seg, layer := range nodes {
		if _, ok := byLayer[layer]; !ok {
			layers = append(layers, layer)
		}
		byLayer[layer] = append(byLayer[layer], seg)
	}
	sort.Strings(layers)
	for _, layer := range layers {
		segs := byLayer[layer]
		sort.Strings(segs)
		fmt.Fprintf(&b, "  subgraph %s\n", sanitizeID(layer))
		fmt.Fprintf(&b, "    direction LR\n")
		for _, seg := range segs {
			fmt.Fprintf(&b, "    %s[\"%s\"]\n", sanitizeID(seg), seg)
		}
		b.WriteString("  end\n")
	}
	// 边（去重排序保证确定性）
	keys := make([]string, 0, len(edges))
	for from := range edges {
		keys = append(keys, from)
	}
	sort.Strings(keys)
	seenEdge := map[string]bool{}
	for _, from := range keys {
		tos := edges[from]
		sort.Strings(tos)
		for _, to := range tos {
			key := from + "->" + to
			if seenEdge[key] {
				continue
			}
			seenEdge[key] = true
			fmt.Fprintf(&b, "  %s --> %s\n", sanitizeID(from), sanitizeID(to))
		}
	}
	return b.String()
}

// sanitizeID 生成合法的 Mermaid 节点 ID（字母数字下划线）。
func sanitizeID(seg string) string {
	var b strings.Builder
	for _, r := range seg {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// RouteTable 把路由表按 domain 分组渲染为文本表格行。
func RouteTable(f facts.Facts) []string {
	var out []string
	lastDomain := ""
	for _, r := range f.Go.Routes {
		if r.Domain != lastDomain {
			out = append(out, "── "+r.Domain+" ──")
			lastDomain = r.Domain
		}
		out = append(out, fmt.Sprintf("%-6s %s", r.Method, r.Path))
	}
	return out
}
