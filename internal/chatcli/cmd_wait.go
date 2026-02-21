package chatcli

import (
	"fmt"
	"strings"
	"time"

	"agent-toolkit/internal/chatd"
	"github.com/spf13/cobra"
)

var (
	waitAgent   string
	waitThread  string
	waitTimeout string
)

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for the next message in an agent inbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(waitAgent) == "" {
			return fmt.Errorf("--agent is required")
		}

		timeout := chatd.DefaultClientTimeout
		if strings.TrimSpace(waitTimeout) != "" {
			parsed, err := time.ParseDuration(waitTimeout)
			if err != nil {
				return fmt.Errorf("invalid --timeout value: %w", err)
			}
			if parsed < chatd.MinWaitTimeout || parsed > chatd.MaxWaitTimeout {
				return fmt.Errorf("--timeout must be between %s and %s", chatd.MinWaitTimeout, chatd.MaxWaitTimeout)
			}
			timeout = parsed
		}

		cfg, err := resolveConfig(serverURLFlag, dbPathFlag)
		if err != nil {
			return err
		}
		if err := ensureDaemon(cfg); err != nil {
			return err
		}

		payload := chatd.WaitRequest{
			Agent:    waitAgent,
			ThreadID: waitThread,
			Timeout:  timeout.String(),
		}

		var resp map[string]any
		statusCode, err := postJSON(cfg.ServerURL, "/v1/messages/wait", payload, &resp)
		if err != nil {
			return err
		}
		if statusCode != 200 {
			return apiError(statusCode, resp)
		}

		return outputJSON(resp)
	},
}

func init() {
	waitCmd.Flags().StringVar(&waitAgent, "agent", "", "Agent inbox to poll")
	waitCmd.Flags().StringVar(&waitThread, "thread", "", "Optional thread identifier")
	waitCmd.Flags().StringVar(&waitTimeout, "timeout", chatd.DefaultClientTimeout.String(), "Wait timeout (1s to 15m)")
	_ = waitCmd.MarkFlagRequired("agent")
}
