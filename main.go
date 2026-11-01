package main

import (
	"errors"
	"os"

	"revy/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if errors.Is(err, cli.ErrExitFail) {
			os.Exit(1) // 契约：存在未通过的策略
		}
		os.Exit(2) // 运行错误
	}
}
