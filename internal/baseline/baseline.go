// Package baseline 实现基线豁免：现存失败记入基线后降级为 warn，
// 只对新增失败按原严重度处理（golangci new-from-rev 思路）。
package baseline

import (
	"encoding/json"
	"os"
)

// Baseline 记录已被豁免的策略 ID 列表。
type Baseline struct {
	Suppressed []string `json:"suppressed"`
}

// Load 读取项目根的 .review/baseline.json；不存在返回空基线。
func Load(root string) Baseline {
	b := Baseline{}
	data, err := os.ReadFile(root + "/.review/baseline.json")
	if err != nil {
		return b
	}
	_ = json.Unmarshal(data, &b)
	return b
}

// Suppress b 中记录的策略 ID 集合。
func (b Baseline) Suppress() map[string]struct{} {
	m := make(map[string]struct{}, len(b.Suppressed))
	for _, id := range b.Suppressed {
		m[id] = struct{}{}
	}
	return m
}

// Set 由 ID 列表构造豁免集合。
func Set(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// Save 把当前失败的策略 ID 写入基线（revy baseline update）。
func Save(root string, failedIDs []string) error {
	data, err := json.MarshalIndent(Baseline{Suppressed: failedIDs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root+"/.review", 0o755); err != nil {
		return err
	}
	return os.WriteFile(root+"/.review/baseline.json", append(data, '\n'), 0o644)
}
