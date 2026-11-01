package review

import (
	"os"
	"path/filepath"
	"testing"

	"revy/internal/baseline"
	"revy/internal/policy"
)

// fixture 构造一个最小 Go 模块：go.mod + 一个包 + 一个测试文件。
// goFiles 的键为相对路径，值为文件内容（调用方保证 gofmt 干净）。
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/fixture\n\ngo 1.24\n")
	for rel, content := range files {
		write(rel, content)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

const passingTest = `package fixture

import "testing"

func TestPass(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("math broken")
	}
}
`

const failingTest = `package fixture

import "testing"

func TestFail(t *testing.T) {
	t.Fatal("deliberate failure")
}
`

// 黄金路径：干净模块 → build/vet/test 全过，策略全绿，工件落盘。
func TestRun_GoldenPath(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试")
	}
	root := fixture(t, map[string]string{
		"fixture.go":      "package fixture\n\nfunc Add(a, b int) int { return a + b }\n",
		"fixture_test.go": passingTest,
	})

	res, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.Passed || res.Fail != 0 {
		t.Fatalf("expected pass, got fail=%d", res.Fail)
	}
	if res.ArtifactPath == "" {
		t.Fatal("artifact not written")
	}
	if _, err := os.Stat(res.ArtifactPath); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if !res.Facts.Go.Found {
		t.Fatal("go facts not collected")
	}
}

// 失败路径：测试失败 → go-test-pass 策略 fail，exit 语义由调用方处理。
func TestRun_FailingTests(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试")
	}
	root := fixture(t, map[string]string{
		"fixture.go":      "package fixture\n\nfunc Add(a, b int) int { return a + b }\n",
		"fixture_test.go": failingTest,
	})

	res, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Passed || res.Fail == 0 {
		t.Fatal("expected policy failure")
	}
	found := false
	for _, r := range res.Results {
		if r.Policy.ID == "go-test-pass" && !r.Passed {
			found = true
		}
	}
	if !found {
		t.Fatal("go-test-pass should be among failures")
	}
}

// 基线路径：失败策略记入基线后降级 warn（不进 fail 计数）。
func TestRun_BaselineSuppresses(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试")
	}
	root := fixture(t, map[string]string{
		"fixture.go":      "package fixture\n\nfunc Add(a, b int) int { return a + b }\n",
		"fixture_test.go": failingTest,
	})
	if err := baseline.Save(root, []string{"go-test-pass"}); err != nil {
		t.Fatal(err)
	}

	res, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Fail != 0 {
		t.Fatalf("baseline should suppress failure, got fail=%d", res.Fail)
	}
	suppressedBaseline := baseline.Load(root).Suppress()
	_ = suppressedBaseline
	sup := false
	for _, id := range res.BaselineUsed {
		if id == "go-test-pass" {
			sup = true
		}
	}
	if !sup {
		t.Fatal("go-test-pass should be marked as baseline-suppressed")
	}
}

// 严格模式：基线豁免的 warn 升级为 fail。
func TestRun_StrictUpgradesBaselineWarn(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试")
	}
	root := fixture(t, map[string]string{
		"fixture.go":      "package fixture\n\nfunc Add(a, b int) int { return a + b }\n",
		"fixture_test.go": failingTest,
	})
	if err := baseline.Save(root, []string{"go-test-pass"}); err != nil {
		t.Fatal(err)
	}

	res, err := Run(root, Options{Strict: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Fail == 0 {
		t.Fatal("strict mode should upgrade baseline-suppressed warn to fail")
	}
}

// 事件回调：每个采集阶段至少推送一次 running。
func TestRun_EmitsProgressEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试")
	}
	root := fixture(t, map[string]string{
		"fixture.go": "package fixture\n\nfunc Add(a, b int) int { return a + b }\n",
	})
	var events []ProgressEvent
	res, err := Run(root, Options{OnEvent: func(e ProgressEvent) { events = append(events, e) }})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Pass == 0 {
		t.Fatal("expected some passing policies")
	}
	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	sawPolicy := false
	for _, e := range events {
		if e.Stage == "policy" {
			sawPolicy = true
		}
	}
	if !sawPolicy {
		t.Fatal("expected a policy-stage event")
	}
}

// 策略加载：预置包可加载且可编译（防手滑写坏内置包）。
func TestPresetPolicies(t *testing.T) {
	ps, err := policy.LoadPreset()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) < 20 {
		t.Fatalf("strict pack should have >=20 policies, got %d", len(ps))
	}
	if errs := policy.Validate(ps); len(errs) > 0 {
		for _, e := range errs {
			t.Error(e)
		}
	}
}
