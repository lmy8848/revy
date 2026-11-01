// Package policy 实现 CEL 策略引擎：加载 YAML 策略声明，对 facts 求值。
package policy

import (
	"fmt"
	"os"
	"strings"

	"cel.dev/cel-go/cel"
	"gopkg.in/yaml.v3"
)

// Severity 等级：error / warn / info。
type Severity string

const (
	SevError Severity = "error"
	SevWarn  Severity = "warn"
	SevInfo  Severity = "info"
)

// Policy 单条策略：对 facts 的一个布尔判定 + 语义说明。
type Policy struct {
	ID        string   `yaml:"id" json:"id"`
	Dimension string   `yaml:"dimension" json:"dimension"` // ci / security / standards / tests / architecture / deps / docs
	Severity  Severity `yaml:"severity" json:"severity"`
	Expr      string   `yaml:"expr" json:"expr"`       // CEL 表达式，值为 bool
	Message   string   `yaml:"message" json:"message"` // 失败时展示的说明
}

// Load 从目录加载全部 *.yaml / *.yml 策略文件，按 ID 去重（后加载者覆盖）。
func Load(dirs ...string) ([]Policy, error) {
	byID := map[string]Policy{}
	var order []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // 目录不存在 = 无该来源策略
		}
		for _, e := range entries {
			if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
				continue
			}
			data, err := os.ReadFile(dir + "/" + e.Name())
			if err != nil {
				return nil, fmt.Errorf("读取策略 %s: %w", e.Name(), err)
			}
			var list struct {
				Policies []Policy `yaml:"policies"`
			}
			if err := yaml.Unmarshal(data, &list); err != nil {
				return nil, fmt.Errorf("解析策略 %s: %w", e.Name(), err)
			}
			for _, p := range list.Policies {
				if p.ID == "" || p.Expr == "" {
					return nil, fmt.Errorf("策略 %s/%s 缺少 id 或 expr", e.Name(), p.ID)
				}
				if p.Severity == "" {
					p.Severity = SevError
				}
				if _, seen := byID[p.ID]; !seen {
					order = append(order, p.ID)
				}
				byID[p.ID] = p
			}
		}
	}
	out := make([]Policy, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

// Validate 校验全部策略表达式可编译（revy policies validate）。
func Validate(policies []Policy) []error {
	var errs []error
	for _, p := range policies {
		env, err := cel.NewEnv(cel.Variable("facts", cel.MapType(cel.StringType, cel.DynType)))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.ID, err))
			continue
		}
		if _, iss := env.Compile(p.Expr); iss.Err() != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.ID, iss.Err()))
		}
	}
	return errs
}

// Evaluate 对 facts 求值全部策略，返回逐条结果。
func Evaluate(policies []Policy, facts map[string]any) []Result {
	results := make([]Result, 0, len(policies))
	for _, p := range policies {
		res := Result{Policy: p, Passed: false} // 出错视同失败，绝不静默放行
		env, err := cel.NewEnv(cel.Variable("facts", cel.MapType(cel.StringType, cel.DynType)))
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		ast, iss := env.Compile(p.Expr)
		if iss.Err() != nil {
			res.Error = iss.Err().Error()
			results = append(results, res)
			continue
		}
		prg, err := env.Program(ast)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		out, _, terr := prg.Eval(map[string]any{"facts": facts})
		if terr != nil {
			res.Error = terr.Error()
			results = append(results, res)
			continue
		}
		b, ok := out.Value().(bool)
		if !ok {
			res.Error = fmt.Sprintf("表达式结果非布尔：%v", out.Value())
			results = append(results, res)
			continue
		}
		res.Passed = b
		results = append(results, res)
	}
	return results
}

// Result 单条策略的求值结果。
type Result struct {
	Policy Policy `json:"policy"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"` // 求值错误（表达式异常），视同失败
}
