package collect

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"revy/internal/config"
	"revy/internal/facts"
)

// Go 适配器：build / vet / test / fmt / lint（golangci-lint，未安装则跳过）
// + import 依赖图（架构策略的数据源）。
func Go(ctx context.Context, root string, cfg *config.Config, f *facts.Facts) {
	if !fileExists(filepath.Join(root, "go.mod")) {
		return
	}
	f.Project.Types = append(f.Project.Types, "go")
	f.Go.Found = true
	f.Go.GoVersion = goModVersion(root)

	env := environ(cfg.Go.Env)
	timeout := cfg.Timeout()

	build := runCheck{Name: "go build", Cmd: "go", Args: []string{"build", "./..."}, Dir: root, Env: env, Timeout: timeout}
	f.Go.Build.OK, _, f.Go.Build.Output, f.Go.Build.DurationMS = build.run(ctx)

	vet := runCheck{Name: "go vet", Cmd: "go", Args: []string{"vet", "./..."}, Dir: root, Env: env, Timeout: timeout}
	f.Go.Vet.OK, _, f.Go.Vet.Output, f.Go.Vet.DurationMS = vet.run(ctx)

	// 目标平台与宿主不同（如 Windows 宿主审查 Linux 项目）时，
	// go test 无法执行 Linux 二进制——显式跳过并标注，真 Linux 测试
	// 交给 ci 采集器（权威）或 ssh/vm 执行器
	targetGOOS := envValue(env, "GOOS")
	if targetGOOS != "" && targetGOOS != goos() {
		f.Go.Test.OK = false
		f.Go.Test.Skipped = true
		f.Go.Test.SkipReason = "cross-platform: 目标平台 " + targetGOOS + " 与宿主不同，测试由 CI/远端执行器承接"
	} else {
		test := runCheck{Name: "go test", Cmd: "go", Args: []string{"test", "-count=1", "./..."}, Dir: root, Env: env, Timeout: timeout}
		f.Go.Test.OK, _, f.Go.Test.Output, f.Go.Test.DurationMS = test.run(ctx)
	}

	f.Go.Fmt = gofmtDrift(ctx, root)

	lint := runCheck{Name: "golangci-lint", Cmd: "golangci-lint", Args: []string{"run"}, Dir: root, Timeout: timeout}
	f.Go.Lint.OK, f.Go.Lint.Skipped, f.Go.Lint.Output, f.Go.Lint.DurationMS = lint.run(ctx)

	f.Go.ImportGraph = goImportGraph(ctx, root)
	f.Go.Routes = scanGoRoutes(ctx, root)
}

func gofmtDrift(ctx context.Context, root string) facts.FmtFacts {
	if _, err := exec.LookPath("gofmt"); err != nil {
		return facts.FmtFacts{OK: true} // gofmt 缺失时不误报漂移
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := execCommand(cctx, "gofmt", root, "-l", ".")
	f := facts.FmtFacts{OK: err == nil && strings.TrimSpace(out) == ""}
	if strings.TrimSpace(out) != "" {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) > 50 {
			lines = lines[:50]
		}
		f.Drift = lines
	}
	return f
}

// goImportGraph 解析 internal/<pkg> 层级间的内部依赖边。
func goImportGraph(ctx context.Context, root string) facts.ImportGraph {
	ig := facts.ImportGraph{Edges: []facts.ImportEdge{}}
	listOut, err := execCommand(ctx, "go", root, "go", "list", "-json", "./internal/...")
	if err != nil {
		return ig
	}
	dec := json.NewDecoder(strings.NewReader(listOut))
	for dec.More() {
		var pkg struct {
			ImportPath string   `json:"ImportPath"`
			Imports    []string `json:"Imports"`
		}
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		from := internalSegment(pkg.ImportPath)
		if from == "" {
			continue
		}
		seen := map[string]bool{}
		for _, imp := range pkg.Imports {
			if !strings.HasPrefix(imp, moduleName(root)) || !strings.Contains(imp, "/internal/") {
				continue
			}
			to := internalSegment(imp)
			if to == "" || to == from || seen[to] {
				continue
			}
			seen[to] = true
			ig.Edges = append(ig.Edges, facts.ImportEdge{From: from, To: to})
		}
	}
	return ig
}

// internalSegment 取 internal/ 后的第一段，如
// easyserver/internal/domain/auth -> domain/auth。
func internalSegment(importPath string) string {
	i := strings.Index(importPath, "/internal/")
	if i < 0 {
		return ""
	}
	return importPath[i+len("/internal/"):]
}

func moduleName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func goModVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	return ""
}

func environ(overrides map[string]string) []string {
	env := os.Environ()
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}
