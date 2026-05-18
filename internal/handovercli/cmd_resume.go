package handovercli

import (
	"io"
	"os"
	"path/filepath"

	"agent-toolkit/internal/shared/cliio"
	"github.com/spf13/cobra"
)

var resumeOpts struct {
	codexBin       string
	outputLast     string
	timeoutSeconds int
}

var resumeCmd = &cobra.Command{
	Use:   "resume SESSION_ID [PROMPT|-]",
	Short: "Resume a kicked-off Codex handover session",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]
		prompt := ""
		if len(args) > 1 {
			prompt = args[1]
		}
		if prompt == "-" || (prompt == "" && stdinHasData()) {
			content, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			prompt = string(content)
		}
		if prompt == "" {
			prompt = "Continue from the handover briefing."
		}

		outputLast := resumeOpts.outputLast
		if outputLast == "" {
			outputLast = filepath.Join(os.TempDir(), "codex-handover-resume-last-message.txt")
		}
		ctx, cancel := timeoutContext(resumeOpts.timeoutSeconds)
		defer cancel()
		result, err := RunCodexResume(ctx, resumeOpts.codexBin, sessionID, prompt, outputLast)
		if err != nil {
			return err
		}
		return cliio.OutputJSON(map[string]any{
			"status":              "success",
			"action":              "resume",
			"session_id":          sessionID,
			"thread_url":          result.ThreadURL,
			"thread_url_status":   result.ThreadURLStatus,
			"pickup_command":      result.PickupCommand,
			"output_last_message": outputLast,
		})
	},
}

func init() {
	resumeCmd.Flags().StringVar(&resumeOpts.codexBin, "codex-bin", "codex", "Codex CLI binary")
	resumeCmd.Flags().StringVar(&resumeOpts.outputLast, "output-last-message", "", "Path for Codex last message")
	resumeCmd.Flags().IntVar(&resumeOpts.timeoutSeconds, "timeout-seconds", 1800, "Resume timeout in seconds")
}

func stdinHasData() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
