package collect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"revy/internal/config"
	"revy/internal/facts"
)

// WebTS 适配器：web/ 或根目录下的 package.json 项目，
// 依次执行 scripts 里存在的 typecheck / lint / test。
func WebTS(ctx context.Context, root string, cfg *config.Config, f *facts.Facts) {
	dir := findWebDir(root)
	if dir == "" {
		return
	}
	f.Project.Types = append(f.Project.Types, "web-ts")
	f.WebTS.Found = true
	f.WebTS.Dir = dir

	scripts := pkgScripts(dir)
	timeout := cfg.Timeout()

	runScript := func(name string, res *facts.CheckResult) {
		if _, ok := scripts[name]; !ok {
			res.OK = true
			res.Skipped = true
			res.SkipReason = "package.json 无 " + name + " script"
			return
		}
		rc := runCheck{
			Name: "npm run " + name, Cmd: npmCmd(), Args: append([]string{"run", name}, npmArgs()...),
			Dir: dir, Timeout: timeout,
		}
		res.OK, res.Skipped, res.Output, res.DurationMS = rc.run(ctx)
	}
	runScript("typecheck", &f.WebTS.Typecheck)
	runScript("lint", &f.WebTS.Lint)
	runScript("test", &f.WebTS.Test)

	f.WebTS.APICalls = scanTSAPICalls(ctx, dir)
}

func findWebDir(root string) string {
	if fileExists(filepath.Join(root, "package.json")) {
		return root
	}
	if fileExists(filepath.Join(root, "web", "package.json")) {
		return filepath.Join(root, "web")
	}
	return ""
}

func pkgScripts(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	return pkg.Scripts
}

// npmCmd Windows 上是 npm.cmd / pnpm.cmd，Linux 上是 npm/pnpm；
// 优先用 pnpm（存在 pnpm-lock.yaml 时），否则 npm。
func npmCmd() string {
	if runtimeGOOS() == "windows" {
		if _, err := os.Stat("pnpm-lock.yaml"); err == nil {
			if lookPathOK("pnpm.cmd") {
				return "pnpm.cmd"
			}
		}
		if lookPathOK("npm.cmd") {
			return "npm.cmd"
		}
	}
	if lookPathOK("pnpm") {
		return "pnpm"
	}
	return "npm"
}

func npmArgs() []string { return nil }

func runtimeGOOS() string { return goos() }

// Flutter 适配器：pubspec.yaml 项目，flutter analyze / flutter test。
func Flutter(ctx context.Context, root string, cfg *config.Config, f *facts.Facts) {
	if !fileExists(filepath.Join(root, "pubspec.yaml")) {
		return
	}
	f.Project.Types = append(f.Project.Types, "flutter")
	f.Flutter.Found = true
	timeout := cfg.Timeout()

	a := runCheck{Name: "flutter analyze", Cmd: "flutter", Args: []string{"analyze"}, Dir: root, Timeout: timeout}
	f.Flutter.Analyze.OK, f.Flutter.Analyze.Skipped, f.Flutter.Analyze.Output, f.Flutter.Analyze.DurationMS = a.run(ctx)

	t := runCheck{Name: "flutter test", Cmd: "flutter", Args: []string{"test"}, Dir: root, Timeout: timeout}
	f.Flutter.Test.OK, f.Flutter.Test.Skipped, f.Flutter.Test.Output, f.Flutter.Test.DurationMS = t.run(ctx)
}
