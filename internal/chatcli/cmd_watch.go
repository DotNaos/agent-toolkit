package chatcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"agent-toolkit/internal/chatd"
	"github.com/spf13/cobra"
)

var (
	watchAgent       string
	watchThread      string
	watchTimeout     string
	watchIdleTimeout string
	watchHandler     string
	watchAutoAck     bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run a long-lived watcher that waits for messages and optionally handles them",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(watchAgent) == "" {
			return fmt.Errorf("--agent is required")
		}

		waitTimeout, err := parseDurationInRange(watchTimeout, chatd.MinWaitTimeout, chatd.MaxWaitTimeout, "--timeout")
		if err != nil {
			return err
		}

		idleTimeout := time.Duration(0)
		if strings.TrimSpace(watchIdleTimeout) != "" {
			parsed, err := time.ParseDuration(watchIdleTimeout)
			if err != nil {
				return fmt.Errorf("invalid --idle-timeout value: %w", err)
			}
			if parsed < 0 {
				return fmt.Errorf("--idle-timeout must be >= 0")
			}
			idleTimeout = parsed
		}

		cfg, err := resolveConfig(serverURLFlag, dbPathFlag)
		if err != nil {
			return err
		}
		if err := ensureDaemon(cfg); err != nil {
			return err
		}

		lastActivity := time.Now()
		for {
			var waitMap map[string]any
			statusCode, err := postJSON(cfg.ServerURL, "/v1/messages/wait", chatd.WaitRequest{
				Agent:    watchAgent,
				ThreadID: watchThread,
				Timeout:  waitTimeout.String(),
			}, &waitMap)
			if err != nil {
				return err
			}
			if statusCode != 200 {
				return apiError(statusCode, waitMap)
			}

			waitPayloadBytes, err := json.Marshal(waitMap)
			if err != nil {
				return fmt.Errorf("failed to marshal wait payload: %w", err)
			}

			waitResp := chatd.WaitResponse{}
			if err := json.Unmarshal(waitPayloadBytes, &waitResp); err != nil {
				return fmt.Errorf("failed to parse wait payload: %w", err)
			}

			switch waitResp.Status {
			case "timeout":
				if idleTimeout > 0 && time.Since(lastActivity) >= idleTimeout {
					return outputJSON(map[string]any{
						"status":       "success",
						"action":       "watch",
						"result":       "idle-timeout",
						"agent":        watchAgent,
						"thread_id":    watchThread,
						"idle_timeout": idleTimeout.String(),
					})
				}
				continue
			case "success":
				if waitResp.Message == nil {
					return fmt.Errorf("wait response missing message")
				}
				lastActivity = time.Now()
			default:
				return fmt.Errorf("unexpected wait status %q", waitResp.Status)
			}

			event := map[string]any{
				"status":           "success",
				"action":           "watch-received",
				"agent":            watchAgent,
				"thread_id":        waitResp.Message.ThreadID,
				"message":          waitResp.Message,
				"lease_token":      waitResp.LeaseToken,
				"lease_expires_at": waitResp.LeaseExpiresAt,
			}
			if err := outputJSON(event); err != nil {
				return err
			}

			if strings.TrimSpace(watchHandler) != "" {
				if err := runWatchHandler(watchHandler, watchAgent, waitResp); err != nil {
					_ = outputJSON(map[string]any{
						"status":     "error",
						"action":     "watch-handler",
						"message_id": waitResp.Message.ID,
						"message":    err.Error(),
					})
					continue
				}
			} else if !watchAutoAck {
				continue
			}

			ackResp := map[string]any{}
			statusCode, err = postJSON(cfg.ServerURL, "/v1/messages/ack", chatd.AckRequest{
				Agent:      watchAgent,
				MessageID:  waitResp.Message.ID,
				LeaseToken: waitResp.LeaseToken,
			}, &ackResp)
			if err != nil {
				return err
			}
			if statusCode != 200 {
				return apiError(statusCode, ackResp)
			}

			if err := outputJSON(map[string]any{
				"status":     "success",
				"action":     "watch-acked",
				"message_id": waitResp.Message.ID,
			}); err != nil {
				return err
			}
		}
	},
}

func init() {
	watchCmd.Flags().StringVar(&watchAgent, "agent", "", "Agent inbox to watch")
	watchCmd.Flags().StringVar(&watchThread, "thread", "", "Optional thread identifier")
	watchCmd.Flags().StringVar(&watchTimeout, "timeout", chatd.DefaultClientTimeout.String(), "Per-wait timeout (1s to 15m)")
	watchCmd.Flags().StringVar(&watchIdleTimeout, "idle-timeout", "0s", "Exit after this idle time (0s = never)")
	watchCmd.Flags().StringVar(&watchHandler, "handler", "", "Optional shell command to execute for each message (stdin gets message JSON)")
	watchCmd.Flags().BoolVar(&watchAutoAck, "auto-ack", false, "Acknowledge message even without --handler")
	_ = watchCmd.MarkFlagRequired("agent")
}

func parseDurationInRange(raw string, min time.Duration, max time.Duration, flagName string) (time.Duration, error) {
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value: %w", flagName, err)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %s and %s", flagName, min, max)
	}
	return parsed, nil
}

func runWatchHandler(handler string, agent string, waitResp chatd.WaitResponse) error {
	msg := waitResp.Message
	if msg == nil {
		return fmt.Errorf("missing message")
	}

	messageJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message for handler: %w", err)
	}

	cmd := exec.Command("zsh", "-lc", handler)
	cmd.Stdin = bytes.NewReader(messageJSON)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"AGENT_CHAT_AGENT="+agent,
		"AGENT_CHAT_MESSAGE_ID="+msg.ID,
		"AGENT_CHAT_FROM_AGENT="+msg.FromAgent,
		"AGENT_CHAT_TO_AGENT="+msg.ToAgent,
		"AGENT_CHAT_THREAD_ID="+msg.ThreadID,
		"AGENT_CHAT_LEASE_TOKEN="+waitResp.LeaseToken,
		"AGENT_CHAT_LEASE_EXPIRES_AT="+waitResp.LeaseExpiresAt,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("handler failed: %w", err)
	}

	return nil
}
