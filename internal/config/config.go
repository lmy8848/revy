// Package config 定义项目级配置（.review/config.yaml）。
package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GitHub GitHubConfig `yaml:"github"`
	Go     GoConfig     `yaml:"go"`
	Checks ChecksConfig `yaml:"checks"`
}

type GitHubConfig struct {
	// Repo 显式指定 owner/name；缺省从 git remote origin 解析。
	Repo string `yaml:"repo"`
}

type GoConfig struct {
	// Env 构建环境覆盖，如 GOOS: linux（审查 Linux 目标项目时交叉构建）。
	Env map[string]string `yaml:"env"`
}

type ChecksConfig struct {
	// TimeoutSeconds 单条检查超时（默认 180）。
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

func (c *Config) Timeout() time.Duration {
	if c.Checks.TimeoutSeconds <= 0 {
		return 180 * time.Second
	}
	return time.Duration(c.Checks.TimeoutSeconds) * time.Second
}

// Load 读取项目根的 .review/config.yaml；不存在返回默认配置。
func Load(root string) *Config {
	c := &Config{}
	data, err := os.ReadFile(root + "/.review/config.yaml")
	if err == nil {
		_ = yaml.Unmarshal(data, c)
	}
	return c
}
