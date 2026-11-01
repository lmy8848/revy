package policy

import (
	_ "embed"
)

//go:embed preset/strict.yaml
var presetStrict string

// PresetStrict 返回内置 strict 策略包（YAML 文本）。
func PresetStrict() string { return presetStrict }
