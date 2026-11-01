package policy

import (
	"os"
	"path/filepath"
	"testing"
)

// 回归：CEL 对嵌套 map 的索引/选择语法（策略包大量使用）。
func TestEvaluate_NestedFacts(t *testing.T) {
	facts := map[string]any{
		"go": map[string]any{
			"found": true,
			"build": map[string]any{"ok": true, "skipped": false},
			"fmt":   map[string]any{"ok": false},
			"import_graph": map[string]any{
				"edges": []any{
					map[string]any{"from": "domain/auth", "to": "infra/config"},
				},
			},
		},
		"github": map[string]any{"found": false},
	}

	cases := []struct {
		name string
		expr string
		want bool
	}{
		{"括号索引两层", "facts['go']['build']['ok']", true},
		{"exists+startsWith", "!facts['go']['import_graph']['edges'].exists(e, e['from'].startsWith('domain/') && e['to'].startsWith('infra/'))", false},
		{"found 短路", "facts['github']['found'] == false || facts['github']['code_scanning']['open_total'] == 0", true},
		{"算术组合", "facts['go']['build']['ok'] && !facts['go']['build']['skipped']", true},
	}
	for _, tc := range cases {
		rs := Evaluate([]Policy{{ID: tc.name, Expr: tc.expr}}, facts)
		if len(rs) != 1 {
			t.Fatalf("%s: expected 1 result", tc.name)
		}
		if rs[0].Error != "" {
			t.Errorf("%s: eval error: %s", tc.name, rs[0].Error)
			continue
		}
		if rs[0].Passed != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, rs[0].Passed, tc.want)
		}
	}
}

// 回归：表达式编译失败必须作为 Error 捕获，而不是 panic。
func TestEvaluate_CompileError(t *testing.T) {
	rs := Evaluate([]Policy{{ID: "bad", Expr: "facts['x'] +"}}, map[string]any{})
	if rs[0].Error == "" {
		t.Fatal("expected compile error captured")
	}
	if rs[0].Passed {
		t.Fatal("compile error must not pass")
	}
}

// 回归：全部预置策略可编译（防手滑写坏内置包）。
func TestPresetStrict_Compiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "strict.yaml"), []byte(PresetStrict()), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) < 20 {
		t.Fatalf("strict 包应不少于 20 条策略，实际 %d", len(ps))
	}
	if errs := Validate(ps); len(errs) > 0 {
		for _, e := range errs {
			t.Error(e)
		}
	}
}
