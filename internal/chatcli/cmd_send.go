package chatcli

import (
	"fmt"
	"strings"

	"agent-toolkit/internal/chatd"
	"github.com/spf13/cobra"
)

var (
	sendToAgent   string
	sendFromAgent string
	sendThreadID  string
	sendBody      string
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to an agent inbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(sendToAgent) == "" {
			return fmt.Errorf("--to is required")
		}
		if strings.TrimSpace(sendBody) == "" {
			return fmt.Errorf("--body is required")
		}

		cfg, err := resolveConfig(serverURLFlag, dbPathFlag)
		if err != nil {
			return err
		}
		if err := ensureDaemon(cfg); err != nil {
			return err
		}

		payload := chatd.SendRequest{
			ToAgent:   sendToAgent,
			FromAgent: sendFromAgent,
			ThreadID:  sendThreadID,
			Body:      sendBody,
		}

		var resp map[string]any
		statusCode, err := postJSON(cfg.ServerURL, "/v1/messages/send", payload, &resp)
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
	sendCmd.Flags().StringVar(&sendToAgent, "to", "", "Destination agent")
	sendCmd.Flags().StringVar(&sendFromAgent, "from", "", "Sender agent")
	sendCmd.Flags().StringVar(&sendThreadID, "thread", "", "Optional thread identifier")
	sendCmd.Flags().StringVar(&sendBody, "body", "", "Message body")

	_ = sendCmd.MarkFlagRequired("to")
	_ = sendCmd.MarkFlagRequired("body")
}
