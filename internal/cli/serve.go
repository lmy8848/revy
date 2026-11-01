package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"revy/internal/report"
	"revy/internal/review"
)

//go:embed serve_index.html
var serveIndex string

var serveAddr string
var serveNoOpen bool

var serveCmd = &cobra.Command{
	Use:   "serve [path]",
	Short: "本地仪表盘：触发审查、查看报告/架构图/历史",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot(cmd, args)
		addr := serveAddr
		if addr == "" {
			addr = "127.0.0.1:9366"
		}

		mux := http.NewServeMux()

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, serveIndex)
		})

		mux.HandleFunc("/api/run/stream", func(w http.ResponseWriter, r *http.Request) {
			flusher, canFlush := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			if canFlush {
				w.WriteHeader(http.StatusOK)
				flusher.Flush()
			}
			rootDir := r.URL.Query().Get("root")
			strict := r.URL.Query().Get("strict") == "true"

			artifact, err := review.RunWithContext(r.Context(), rootDir, review.Options{
				Strict: strict,
				OnEvent: func(ev review.ProgressEvent) {
					data, _ := json.Marshal(ev)
					_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
					if canFlush {
						flusher.Flush()
					}
				},
			})

			final := map[string]any{"ok": err == nil}
			if err != nil {
				final["error"] = err.Error()
			} else {
				final["passed"] = artifact.Passed
				final["pass"], final["warn"], final["fail"] = artifact.Pass, artifact.Warn, artifact.Fail
				final["policies"] = artifact.Results
				final["trend"] = artifact.Trend
				final["artifact"] = artifact.ArtifactPath
				final["report"] = artifact.ReportPath
			}
			data, _ := json.Marshal(final)
			_, _ = fmt.Fprintf(w, "event: final\ndata: %s\n\n", data)
			if canFlush {
				flusher.Flush()
			}
		})

		mux.HandleFunc("/api/run", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Root   string `json:"root"`
				Strict bool   `json:"strict"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			rootDir := req.Root
			if rootDir == "" {
				rootDir = root
			}
			artifact, err := review.Run(rootDir, review.Options{Strict: req.Strict})
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if err != nil && artifact.Pass == 0 && artifact.Fail == 0 {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(artifact)
		})

		mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
			rootDir := r.URL.Query().Get("root")
			if rootDir == "" {
				rootDir = root
			}
			runsDir := filepath.Join(rootDir, ".review", "runs")
			entries, err := os.ReadDir(runsDir)
			type runItem struct {
				File string    `json:"file"`
				Time time.Time `json:"time"`
			}
			out := []runItem{}
			if err == nil {
				for _, e := range entries {
					if !strings.HasPrefix(e.Name(), "run-") {
						continue
					}
					ts, perr := time.Parse("20060102-150405",
						strings.TrimSuffix(strings.TrimPrefix(e.Name(), "run-"), ".json"))
					if perr == nil {
						out = append(out, runItem{File: e.Name(), Time: ts})
					}
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(out)
		})

		mux.HandleFunc("/api/artifact", func(w http.ResponseWriter, r *http.Request) {
			file := filepath.Base(r.URL.Query().Get("file"))
			rootDir := r.URL.Query().Get("root")
			if rootDir == "" {
				rootDir = root
			}
			data, err := os.ReadFile(filepath.Join(rootDir, ".review", "runs", file))
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(data)
		})

		mux.HandleFunc("/report/", func(w http.ResponseWriter, r *http.Request) {
			file := filepath.Base(r.URL.Path)
			rootDir := r.URL.Query().Get("root")
			if rootDir == "" {
				rootDir = root
			}
			http.ServeFile(w, r, filepath.Join(rootDir, ".review", "reports", file))
		})

		url := "http://" + addr
		fmt.Printf("revy 仪表盘: %s （项目: %s）\n按 Ctrl+C 停止\n", url, root)

		if !serveNoOpen {
			openBrowser(url)
		}
		return http.ListenAndServe(addr, mux)
	},
}

var reportCmd = &cobra.Command{
	Use:   "report <artifact.json>",
	Short: "从运行工件回放渲染报告（terminal/html）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		var art review.ReviewResult
		if err := json.Unmarshal(data, &art); err != nil {
			return fmt.Errorf("解析工件: %w", err)
		}
		run := artifactToRun(art)
		switch format {
		case "html":
			html, err := report.HTMLWithTrend(&run, art.Trend.New, art.Trend.Resolved)
			if err != nil {
				return err
			}
			out := strings.TrimSuffix(args[0], ".json") + ".html"
			if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
				return err
			}
			fmt.Println("已生成", out)
		default:
			fmt.Print(report.Terminal(&run))
		}
		return nil
	},
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "", "监听地址（默认 127.0.0.1:9366）")
	serveCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "不自动打开浏览器")
	rootCmd.AddCommand(serveCmd)

	reportCmd.Flags().StringP("format", "f", "terminal", "输出格式：terminal | html")
	rootCmd.AddCommand(reportCmd)
}
