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
	// TestSSH 把 go test 派发到 Linux 远端执行（Windows 宿主无法运行
	// Linux 二进制时使用；cross-platform 跳过的替代方案）。
	TestSSH bool      `yaml:"test_ssh"`
	SSH     SSHConfig `yaml:"ssh"`
	// Modules 额外扫描的子模块目录（如 desktop）；缺省自动发现全部 go.mod。
	Modules []string `yaml:"modules"`
}

// SSHConfig 远端执行器连接参数。
type SSHConfig struct {
	Host string `yaml:"host"` // user@host
	Key  string `yaml:"key"`  // 私钥路径；空则用默认凭据
	Dir  string `yaml:"dir"`  // 远端工作目录（项目将同步到其下）
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
