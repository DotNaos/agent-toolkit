package chatcli

import (
	"fmt"
	"strings"

	"agent-toolkit/internal/chatd"
	"github.com/spf13/cobra"
)

var (
	ackAgent      string
	ackMessageID  string
	ackLeaseToken string
)

var ackCmd = &cobra.Command{
	Use:   "ack",
	Short: "Acknowledge a leased message",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(ackAgent) == "" {
			return fmt.Errorf("--agent is required")
		}
		if strings.TrimSpace(ackMessageID) == "" {
			return fmt.Errorf("--id is required")
		}
		if strings.TrimSpace(ackLeaseToken) == "" {
			return fmt.Errorf("--lease-token is required")
		}

		cfg, err := resolveConfig(serverURLFlag, dbPathFlag)
		if err != nil {
			return err
		}
		if err := ensureDaemon(cfg); err != nil {
			return err
		}

		payload := chatd.AckRequest{
			Agent:      ackAgent,
			MessageID:  ackMessageID,
			LeaseToken: ackLeaseToken,
		}

		var resp map[string]any
		statusCode, err := postJSON(cfg.ServerURL, "/v1/messages/ack", payload, &resp)
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
	ackCmd.Flags().StringVar(&ackAgent, "agent", "", "Agent inbox owner")
	ackCmd.Flags().StringVar(&ackMessageID, "id", "", "Message ID")
	ackCmd.Flags().StringVar(&ackLeaseToken, "lease-token", "", "Lease token from wait")

	_ = ackCmd.MarkFlagRequired("agent")
	_ = ackCmd.MarkFlagRequired("id")
	_ = ackCmd.MarkFlagRequired("lease-token")
}
