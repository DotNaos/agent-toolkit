package handovercli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-toolkit/internal/shared/cliio"
	"github.com/spf13/cobra"
)

var kickoffOpts struct {
	briefing         string
	targetProject    string
	codexBin         string
	sandbox          string
	outputLast       string
	timeoutSeconds   int
	dryRun           bool
	skipGitRepoCheck bool
	runtime          string
	title            string
	idea             string
	requestedChange  string
	mode             string
}

var kickoffCmd = &cobra.Command{
	Use:   "kickoff",
	Short: "Start a non-interactive Codex session from a handover briefing",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetProject := kickoffOpts.targetProject
		if targetProject == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			targetProject = cwd
		}
		targetProject, _ = filepath.Abs(targetProject)

		briefingPath := kickoffOpts.briefing
		briefing := ""
		if briefingPath != "" {
			content, err := os.ReadFile(briefingPath)
			if err != nil {
				return err
			}
			briefing = string(content)
		} else {
			now := time.Now()
			briefingPath = DefaultBriefingPath(targetProject, kickoffOpts.title, now)
			briefing = BuildBriefing(BriefingOptions{
				Title:           kickoffOpts.title,
				TargetProject:   targetProject,
				Mode:            firstNonEmpty(kickoffOpts.mode, "kickoff"),
				Idea:            kickoffOpts.idea,
				RequestedChange: kickoffOpts.requestedChange,
				CreatedAt:       now,
			})
			if err := os.MkdirAll(filepath.Dir(briefingPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(briefingPath, []byte(briefing), 0o644); err != nil {
				return err
			}
		}

		outputLast := kickoffOpts.outputLast
		if outputLast == "" {
			outputLast = filepath.Join(targetProject, ".codex", "handoffs", "last-kickoff-message.txt")
		}
		if err := os.MkdirAll(filepath.Dir(outputLast), 0o755); err != nil {
			return err
		}

		if kickoffOpts.dryRun {
			return cliio.OutputJSON(map[string]any{
				"status":              "success",
				"action":              "kickoff",
				"dry_run":             true,
				"briefing_path":       briefingPath,
				"target_project":      targetProject,
				"codex_bin":           firstNonEmpty(kickoffOpts.codexBin, "codex"),
				"sandbox":             firstNonEmpty(kickoffOpts.sandbox, "read-only"),
				"runtime":             firstNonEmpty(kickoffOpts.runtime, "desktop"),
				"skip_git_repo_check": kickoffOpts.skipGitRepoCheck,
				"output_last_message": outputLast,
			})
		}

		ctx, cancel := timeoutContext(kickoffOpts.timeoutSeconds)
		defer cancel()
		runOpts := CodexRunOptions{
			CodexBin:          kickoffOpts.codexBin,
			TargetProject:     targetProject,
			Sandbox:           kickoffOpts.sandbox,
			Briefing:          briefing,
			OutputLastMessage: outputLast,
			SkipGitRepoCheck:  kickoffOpts.skipGitRepoCheck,
		}
		runtime := firstNonEmpty(kickoffOpts.runtime, "desktop")
		var result *CodexRunResult
		var err error
		switch runtime {
		case "desktop":
			result, err = RunCodexAppServerKickoff(ctx, runOpts)
		case "exec":
			result, err = RunCodexExec(ctx, runOpts)
		default:
			return fmt.Errorf("--runtime must be desktop or exec")
		}
		if err != nil {
			return err
		}

		return cliio.OutputJSON(map[string]any{
			"status":              "success",
			"action":              "kickoff",
			"briefing_path":       briefingPath,
			"target_project":      targetProject,
			"runtime":             runtime,
			"session_id":          result.SessionID,
			"thread_url":          result.ThreadURL,
			"thread_url_status":   result.ThreadURLStatus,
			"pickup_command":      result.PickupCommand,
			"output_last_message": outputLast,
		})
	},
}

func init() {
	kickoffCmd.Flags().StringVar(&kickoffOpts.briefing, "briefing", "", "Existing briefing file")
	kickoffCmd.Flags().StringVar(&kickoffOpts.targetProject, "target-project", "", "Target project path (default current directory)")
	kickoffCmd.Flags().StringVar(&kickoffOpts.codexBin, "codex-bin", "codex", "Codex CLI binary")
	kickoffCmd.Flags().StringVar(&kickoffOpts.sandbox, "sandbox", "read-only", "Codex sandbox mode")
	kickoffCmd.Flags().StringVar(&kickoffOpts.outputLast, "output-last-message", "", "Path for Codex last message")
	kickoffCmd.Flags().IntVar(&kickoffOpts.timeoutSeconds, "timeout-seconds", 1800, "Kickoff timeout in seconds")
	kickoffCmd.Flags().BoolVar(&kickoffOpts.dryRun, "dry-run", false, "Write or read the briefing without starting Codex")
	kickoffCmd.Flags().BoolVar(&kickoffOpts.skipGitRepoCheck, "skip-git-repo-check", false, "Allow kicking off Codex outside a Git repository")
	kickoffCmd.Flags().StringVar(&kickoffOpts.runtime, "runtime", "desktop", "Session runtime: desktop or exec")
	kickoffCmd.Flags().StringVar(&kickoffOpts.title, "title", "", "Briefing title when --briefing is omitted")
	kickoffCmd.Flags().StringVar(&kickoffOpts.idea, "idea", "", "Idea when --briefing is omitted")
	kickoffCmd.Flags().StringVar(&kickoffOpts.requestedChange, "requested-change", "", "Requested change when --briefing is omitted")
	kickoffCmd.Flags().StringVar(&kickoffOpts.mode, "mode", "kickoff", "Implementation mode when --briefing is omitted")
}
