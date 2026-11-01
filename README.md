# revy

**策略即代码的项目体检台** —— 采集项目事实（facts），以策略脚本（CEL）判定，
输出终端摘要 / JSON / 单文件 HTML 报告。面向智能体友好：全部结果
`--format json` 可结构化消费，退出码 0=通过 1=存在 fail 2=运行错误。

## 特性（v0.1）

- **三层架构**：facts 采集（带 schema 版本）→ CEL 策略判定 → 多视图输出
- **三适配器**：Go（build/vet/test/fmt/golangci/import 依赖图）、web-ts（typecheck/lint/test）、Flutter（analyze/test）
- **GitHub 采集**：Actions 运行状态、code scanning / dependabot / secret scanning 告警计数
- **24 条 strict 预置策略**（九维度）：CI 健康、安全、代码规范、测试、架构纪律、依赖、文档
- **基线豁免**：现存失败记入基线降级 warn，只对新增违规按原严重度处理
- **跨平台宿主**：Windows 宿主审查 Linux 项目时自动交叉构建（`go.env: {GOOS: linux}`），
  测试显式跳过并标注由 CI/远端承接

## 快速开始

```bash
revy init          # 初始化项目 .review/（配置 + 策略目录 + 空基线）
revy run           # 完整审查：终端摘要 + HTML 报告（.review/reports/）
revy run --format json          # 结构化输出（智能体消费）
revy run --strict               # warn 升级为 fail
revy facts       # 只采集 facts，不做策略判定
revy policies    # 校验策略表达式可编译
revy baseline update  # 把当前 fail 写入基线（存量豁免）
```

## 策略

策略 = YAML 声明 + CEL 表达式，输入是版本化的 facts JSON：

```yaml
policies:
  - id: go-build-ok
    dimension: standards   # ci / security / standards / tests / architecture / deps / docs
    severity: error        # error / warn / info
    expr: "facts['go']['found'] == false || facts['go']['build']['ok']"
    message: "go build 失败"
```

- 项目本地策略放 `.review/policies/*.yaml`，与内置 strict 包合并执行（ID 相同时覆盖）
- 非适用维度用 `facts['<段>']['found'] == false || ...` 短路通过
- 表达式对嵌套 map 用括号索引（`facts['a']['b']`），顶层支持点号

## 项目配置（.review/config.yaml）

```yaml
github:
  repo: owner/name    # 留空自动从 git remote 解析
go:
  env: {GOOS: linux}  # 审查 Linux 项目时的构建环境覆盖
checks:
  timeout_seconds: 180
```

## 执行环境语义

- 检查跳过（工具不在 PATH）**不算通过**——核心检查（build/vet/test）在
  go.mod 存在但工具缺失时按 fail 处理并写明原因；可选工具（golangci）跳过降级 warn
- Windows 宿主审查 Linux 项目：build/vet 交叉编译，test 显式跳过
  （`cross-platform`），真 Linux 测试由 CI 采集器（权威）或后续的 ssh/vm 执行器承接

## 路线图

- v0.2 架构图（Mermaid）+ 运行工件回放 + `vm` 执行器（内置 QEMU 微型 Linux 沙箱）
- v0.3 本地实时 UI（SSE 同步 CLI 运行）
- v0.4 MCP server（智能体原生接入）

## 许可

Apache-2.0
