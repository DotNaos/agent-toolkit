package handovercli

import (
	"fmt"
	"os/exec"
	"runtime"

	"agent-toolkit/internal/shared/cliio"
	"github.com/spf13/cobra"
)

var openOpts struct {
	printOnly bool
	force     bool
	stateDB   string
}

var openCmd = &cobra.Command{
	Use:   "open SESSION_ID",
	Short: "Open a Codex thread deep link",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]
		url := CodexThreadURL(sessionID)
		dbPath := firstNonEmpty(openOpts.stateDB, DefaultStateDBPath())
		info, lookupErr := LookupThread(dbPath, sessionID)
		if lookupErr == nil && info.Source == "exec" && !openOpts.force {
			return cliio.OutputJSON(map[string]any{
				"status":            "success",
				"action":            "open",
				"session_id":        sessionID,
				"thread_url":        url,
				"thread_url_status": execThreadURLUnsupported,
				"pickup_command":    CodexResumeCommand("codex", sessionID),
				"opened":            false,
			})
		}
		if !openOpts.printOnly {
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("opening Codex deep links is currently implemented for macOS only; use %s", url)
			}
			if err := exec.Command("open", "-g", url).Run(); err != nil {
				return err
			}
		}
		return cliio.OutputJSON(map[string]any{
			"status":            "success",
			"action":            "open",
			"session_id":        sessionID,
			"thread_url":        url,
			"thread_url_status": "opened as a Codex Desktop deep link",
			"opened":            !openOpts.printOnly,
		})
	},
}

func init() {
	openCmd.Flags().BoolVar(&openOpts.printOnly, "print-only", false, "Print the deep link without opening it")
	openCmd.Flags().BoolVar(&openOpts.force, "force", false, "Open the deep link even when the session is known to be a CLI exec session")
	openCmd.Flags().StringVar(&openOpts.stateDB, "state-db", "", "Codex state database path")
}
