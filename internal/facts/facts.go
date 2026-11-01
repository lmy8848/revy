// Package facts 定义审查事实（facts）的 schema：采集层与策略层之间
// 唯一的契约。schema_version 随破坏性变更递增，策略声明兼容版本。
package facts

const SchemaVersion = 1

// Facts 一次审查运行采集到的全部事实。所有顶层段始终存在
// （found=false 表示该适配器不适用），保证策略表达式的索引安全。
type Facts struct {
	SchemaVersion int          `json:"schema_version"`
	GeneratedAt   string       `json:"generated_at"`
	Project       Project      `json:"project"`
	Go            GoFacts      `json:"go"`
	WebTS         WebTSFacts   `json:"web_ts"`
	Flutter       FlutterFacts `json:"flutter"`
	GitHub        GitHubFacts  `json:"github"`
	Docs          DocsFacts    `json:"docs"`
}

type Project struct {
	Root  string   `json:"root"`
	Types []string `json:"types"` // go / web-ts / flutter
}

// CheckResult 通用检查结果。skipped=true 时 ok 恒为 true（跳过不算失败）。
type CheckResult struct {
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skip_reason"`
	Output     string `json:"output,omitempty"` // 失败时截断的输出尾部
	DurationMS int64  `json:"duration_ms"`
}

type GoFacts struct {
	Found       bool         `json:"found"`
	GoVersion   string       `json:"go_version,omitempty"`
	Build       CheckResult  `json:"build"`
	Vet         CheckResult  `json:"vet"`
	Test        CheckResult  `json:"test"`
	Fmt         FmtFacts     `json:"fmt"`
	Lint        CheckResult  `json:"lint"`
	ImportGraph ImportGraph  `json:"import_graph"`
	Routes      []RouteFacts `json:"routes"`
}

// RouteFacts 从 Go 源码扫描到的 HTTP 路由注册（domain 为文件所在
// internal/<第一段>/... 的归属）。
type RouteFacts struct {
	Domain string `json:"domain"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type FmtFacts struct {
	OK    bool     `json:"ok"`
	Drift []string `json:"drift"` // 未格式化的文件
}

type ImportGraph struct {
	Edges []ImportEdge `json:"edges"`
}

type ImportEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type WebTSFacts struct {
	Found     bool        `json:"found"`
	Dir       string      `json:"dir,omitempty"`
	Typecheck CheckResult `json:"typecheck"`
	Lint      CheckResult `json:"lint"`
	Test      CheckResult `json:"test"`
	// APICalls 前端服务层扫描到的 API 调用路径（与后端路由表对照）。
	APICalls []string `json:"api_calls"`
}

type FlutterFacts struct {
	Found   bool        `json:"found"`
	Analyze CheckResult `json:"analyze"`
	Test    CheckResult `json:"test"`
}

type GitHubFacts struct {
	Found          bool         `json:"found"`
	SkipReason     string       `json:"skip_reason,omitempty"`
	Repo           string       `json:"repo,omitempty"`
	Actions        ActionsFacts `json:"actions"`
	CodeScanning   CountFacts   `json:"code_scanning"`
	Dependabot     CountFacts   `json:"dependabot"`
	SecretScanning CountFacts   `json:"secret_scanning"`
}

type ActionsFacts struct {
	Latest             LatestRun `json:"latest"`
	RecentTotal        int       `json:"recent_total"`
	RecentSuccessCount int       `json:"recent_success_count"`
	RecentSuccessRatio float64   `json:"recent_success_ratio"`
}

type LatestRun struct {
	Conclusion string `json:"conclusion"` // success / failure / ...
	HeadSHA    string `json:"head_sha"`
	Name       string `json:"name"`
}

// CountFacts 按严重度的开放告警计数。
type CountFacts struct {
	OpenCritical int `json:"open_critical"`
	OpenHigh     int `json:"open_high"`
	OpenModerate int `json:"open_moderate"`
	OpenTotal    int `json:"open_total"`
}

type DocsFacts struct {
	Readme bool `json:"readme"`
}
