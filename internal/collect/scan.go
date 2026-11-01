package collect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"revy/internal/facts"
)

var (
	// gin 风格路由注册：router.GET("/path", ...) / protected.POST(...)
	goRouteRe = regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\(\s*"([^"]+)"`)
	// 前端服务层 API 调用：api.get('/files') / http.post("/auth/login")
	tsApiRe = regexp.MustCompile(`(?i)\.(?:get|post|put|delete|patch)\(\s*['"` + "`" + `](/[^'"` + "`" + `]+)`)
)

// scanGoRoutes 扫描 internal/ 下所有 Go 文件的 gin 路由注册，
// domain 取 internal/ 后的第一段（领域归属）。
func scanGoRoutes(ctx context.Context, root string) []facts.RouteFacts {
	internalDir := filepath.Join(root, "internal")
	if !dirExists(internalDir) {
		return []facts.RouteFacts{}
	}
	var routes []facts.RouteFacts
	seen := map[string]bool{}

	_ = filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(internalDir, path)
		if err != nil {
			return nil
		}
		seg := strings.Split(filepath.ToSlash(rel), "/")
		domain := seg[0]
		if len(seg) > 1 {
			domain = seg[0] + "/" + seg[1]
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range goRouteRe.FindAllStringSubmatch(string(data), -1) {
			key := domain + "|" + m[1] + "|" + m[2]
			if seen[key] {
				continue
			}
			seen[key] = true
			routes = append(routes, facts.RouteFacts{Domain: domain, Method: m[1], Path: m[2]})
		}
		return nil
	})
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Domain != routes[j].Domain {
			return routes[i].Domain < routes[j].Domain
		}
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	return routes
}

// scanTSAPICalls 扫描前端 src/ 下 .ts/.tsx 文件中的 API 调用路径。
func scanTSAPICalls(ctx context.Context, dir string) []string {
	srcDir := filepath.Join(dir, "src")
	if !dirExists(srcDir) {
		return []string{}
	}
	seen := map[string]bool{}
	var calls []string

	_ = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() ||
			(!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range tsApiRe.FindAllStringSubmatch(string(data), -1) {
			p := m[1]
			if seen[p] {
				continue
			}
			seen[p] = true
			calls = append(calls, p)
		}
		return nil
	})
	sort.Strings(calls)
	return calls
}
