package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"revy/internal/baseline"
	"revy/internal/report"
	"revy/internal/review"
)

var runOutDir string

var runCmd = &cobra.Command{
	Use:   "run [path]",
	Short: "采集事实 → 策略判定 → 输出报告（默认命令）",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReview,
}

func init() {
	runCmd.Flags().StringVar(&runOutDir, "out", "", "HTML 报告输出目录（缺省写到 <root>/.review/reports/）")
	rootCmd.AddCommand(runCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	strict, _ := cmd.Flags().GetBool("strict")
	root := projectRoot(cmd, args)

	artifact, err := review.Run(root, review.Options{Strict: strict})
	if err != nil {
		return err
	}
	run := artifactToRun(artifact)

	switch format {
	case "json":
		_ = json.NewEncoder(os.Stdout).Encode(artifact)
	default:
		fmt.Print(report.Terminal(&run))
		fmt.Printf("  工件: %s\n", artifact.ArtifactPath)
		if artifact.ReportPath != "" {
			fmt.Printf("  HTML 报告: %s\n", artifact.ReportPath)
		}
	}

	if artifact.Fail > 0 {
		return ErrExitFail
	}
	return nil
}

func artifactToRun(a review.ReviewResult) report.Run {
	return report.Run{
		StartedAt:    mustTime(a.StartedAt),
		DurationMS:   a.DurationMS,
		ProjectRoot:  a.ProjectRoot,
		Facts:        a.Facts,
		Results:      a.Results,
		BaselineUsed: baseline.Set(a.BaselineUsed),
		Passed:       a.Passed,
	}
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}
