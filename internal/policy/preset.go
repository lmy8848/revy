package policy

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed preset/strict.yaml
var presetStrict string

// PresetStrict 返回内置 strict 策略包（YAML 文本）。
func PresetStrict() string { return presetStrict }

// LoadPreset 加载内置 strict 策略包（临时目录承载，调用方无需管理文件）。
func LoadPreset() ([]Policy, error) {
	dir, err := os.MkdirTemp("", "revy-preset-*")
	if err != nil {
		return nil, err
	}
	if werr := os.WriteFile(filepath.Join(dir, "strict.yaml"), []byte(presetStrict), 0o644); werr != nil {
		return nil, werr
	}
	defer func() { _ = os.RemoveAll(dir) }()
	return Load(dir)
}
