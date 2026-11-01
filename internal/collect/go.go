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

// Go 适配器：多模块 build / vet / test / fmt / lint（golangci-lint，未安装则跳过）
// + import 依赖图与路由表（架构策略的数据源）。
func Go(ctx context.Context, root string, cfg *config.Config, f *facts.Facts) {
	if !fileExists(filepath.Join(root, "go.mod")) {
		return
	}
	f.Project.Types = append(f.Project.Types, "go")
	f.Go.Found = true
	f.Go.GoVersion = goModVersion(root)

	env := environ(cfg.Go.Env)
	timeout := cfg.Timeout()

	// 多模块发现：根 go.mod + 子目录 go.mod（排除 vendor/node_modules/build 等）
	modules := []string{root}
	modules = append(modules, discoverModules(root)...)

	// 每模块本地交叉 build/vet；汇总到单一 CheckResult（策略口径不变）
	buildOK, vetOK := true, true
	var buildOuts, vetOuts []string
	for _, mod := range modules {
		b := runCheck{Name: "go build", Cmd: "go", Args: []string{"build", "./..."}, Dir: mod, Env: env, Timeout: timeout}
		ok, _, out, dur := b.run(ctx)
		f.Go.Build.DurationMS += dur
		if !ok {
			buildOK = false
			buildOuts = append(buildOuts, "["+filepath.Base(mod)+"]\n"+out)
		}
		vetOut, vetDur := vetRun(ctx, mod, env, timeout)
		f.Go.Vet.DurationMS += vetDur
		if vetOut != "" {
			vetOK = false
			vetOuts = append(vetOuts, "["+filepath.Base(mod)+"]\n"+vetOut)
		}
	}
	f.Go.Build.OK = buildOK
	f.Go.Build.Output = strings.Join(buildOuts, "\n")
	f.Go.Vet.OK = vetOK
	f.Go.Vet.Output = strings.Join(vetOuts, "\n")

	// fmt：逐模块检查
	var driftAll []string
	fmtOK := true
	for _, mod := range modules {
		d := gofmtDrift(ctx, mod)
		if !d.OK {
			fmtOK = false
			driftAll = append(driftAll, d.Drift...)
		}
	}
	f.Go.Fmt = facts.FmtFacts{OK: fmtOK, Drift: driftAll}

	// test：三种模式——ssh 远端真实执行（最高标准）/ 本地同平台执行 / 跨平台显式跳过
	switch {
	case cfg.Go.TestSSH && cfg.Go.SSH.Host != "":
		f.Go.Test = sshTest(ctx, modules, cfg, timeout)
	case envValue(env, "GOOS") != "" && envValue(env, "GOOS") != goos():
		f.Go.Test.OK = false
		f.Go.Test.Skipped = true
		f.Go.Test.SkipReason = "cross-platform: 目标平台与宿主不同，测试由 CI/远端执行器承接"
	default:
		var testOuts []string
		testOK := true
		for _, mod := range modules {
			t := runCheck{Name: "go test", Cmd: "go", Args: []string{"test", "-count=1", "./..."}, Dir: mod, Env: env, Timeout: timeout}
			ok, _, out, dur := t.run(ctx)
			f.Go.Test.DurationMS += dur
			if !ok {
				testOK = false
				testOuts = append(testOuts, "["+filepath.Base(mod)+"]\n"+out)
			}
		}
		f.Go.Test.OK = testOK
		f.Go.Test.Output = strings.Join(testOuts, "\n")
	}

	// lint：golangci 在根模块跑一次（可选工具）
	lint := runCheck{Name: "golangci-lint", Cmd: "golangci-lint", Args: []string{"run"}, Dir: root, Timeout: timeout}
	f.Go.Lint.OK, f.Go.Lint.Skipped, f.Go.Lint.Output, f.Go.Lint.DurationMS = lint.run(ctx)

	f.Go.ImportGraph = goImportGraph(ctx, root)
	f.Go.Routes = scanGoRoutes(ctx, root)
	for _, mod := range modules[1:] {
		f.Go.Routes = append(f.Go.Routes, scanGoRoutes(ctx, mod)...)
	}
}

func vetRun(ctx context.Context, mod string, env []string, timeout time.Duration) (string, int64) {
	v := runCheck{Name: "go vet", Cmd: "go", Args: []string{"vet", "./..."}, Dir: mod, Env: env, Timeout: timeout}
	ok, _, out, dur := v.run(ctx)
	_ = ok
	return out, dur
}

// sshTest 把各模块同步到 Linux 远端执行 go test（真实执行而非交叉跳过）。
func sshTest(ctx context.Context, modules []string, cfg *config.Config, timeout time.Duration) facts.CheckResult {
	res := facts.CheckResult{}
	e := &SSHExecutor{Host: cfg.Go.SSH.Host, Key: cfg.Go.SSH.Key, Dir: cfg.Go.SSH.Dir}
	var outs []string
	ok := true
	for _, mod := range modules {
		remote, err := e.Sync(ctx, mod)
		if err != nil {
			res.Output = "同步到远端失败: " + err.Error()
			res.OK = false
			return res
		}
		cmd := "export PATH=$PATH:/usr/local/go/bin GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=auto && go test -count=1 ./... 2>&1 | tail -40"
		out, err := e.Run(ctx, remote, cmd, timeout)
		outs = append(outs, "["+filepath.Base(mod)+"]\n"+out)
		if err != nil || strings.TrimSpace(out) == "" {
			ok = false
		}
	}
	res.OK = ok
	res.Output = strings.Join(outs, "\n")
	return res
}

// discoverModules 发现项目下的全部 go.mod（排除 .git/vendor/node_modules/build）。
func discoverModules(root string) []string {
	var mods []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".git", "vendor", "node_modules", "build", ".review", "frontend":
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		if fileExists(filepath.Join(path, "go.mod")) {
			mods = append(mods, path)
			return filepath.SkipDir
		}
		return nil
	})
	return mods
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
