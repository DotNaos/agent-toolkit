package handovercli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type appServerMessage struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type appServerThreadStartResult struct {
	Thread struct {
		ID string `json:"id"`
	} `json:"thread"`
}

type appServerTurnCompletedParams struct {
	ThreadID string `json:"threadId"`
	Turn     struct {
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message,omitempty"`
		} `json:"error,omitempty"`
	} `json:"turn"`
}

type appServerAgentDeltaParams struct {
	ThreadID string `json:"threadId"`
	Delta    string `json:"delta"`
}

type appServerItemCompletedParams struct {
	ThreadID string `json:"threadId"`
	Item     struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"item"`
}

func RunCodexAppServerKickoff(ctx context.Context, opts CodexRunOptions) (*CodexRunResult, error) {
	codexBin := firstNonEmpty(opts.CodexBin, "codex")
	sandbox := firstNonEmpty(opts.Sandbox, "read-only")
	if opts.TargetProject == "" {
		return nil, fmt.Errorf("--target-project is required")
	}
	if opts.Briefing == "" {
		return nil, fmt.Errorf("briefing is empty")
	}

	cmd := exec.CommandContext(ctx, codexBin, "app-server", "--listen", "stdio://")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	messages := make(chan appServerMessage)
	readErrs := make(chan error, 1)
	go readAppServerMessages(stdout, messages, readErrs)

	write := func(id int, method string, params any) error {
		payload := map[string]any{
			"id":     id,
			"method": method,
			"params": params,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdin, string(encoded))
		return err
	}

	if err := write(1, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codex-handover",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}); err != nil {
		return nil, err
	}
	if _, err := waitForResponse(ctx, messages, readErrs, 1, &stderr); err != nil {
		return nil, err
	}

	if err := write(2, "thread/start", map[string]any{
		"cwd":                    opts.TargetProject,
		"approvalPolicy":         "never",
		"sandbox":                sandbox,
		"threadSource":           "user",
		"ephemeral":              false,
		"experimentalRawEvents":  false,
		"persistExtendedHistory": false,
	}); err != nil {
		return nil, err
	}
	startMsg, err := waitForResponse(ctx, messages, readErrs, 2, &stderr)
	if err != nil {
		return nil, err
	}
	start := appServerThreadStartResult{}
	if err := json.Unmarshal(startMsg.Result, &start); err != nil {
		return nil, fmt.Errorf("decode thread/start response: %w", err)
	}
	if start.Thread.ID == "" {
		return nil, fmt.Errorf("thread/start returned no thread id")
	}

	if err := write(3, "turn/start", map[string]any{
		"threadId": start.Thread.ID,
		"input": []map[string]any{{
			"type":          "text",
			"text":          opts.Briefing,
			"text_elements": []any{},
		}},
	}); err != nil {
		return nil, err
	}
	if _, err := waitForResponse(ctx, messages, readErrs, 3, &stderr); err != nil {
		return nil, err
	}

	lastMessage, err := waitForTurnCompleted(ctx, messages, readErrs, start.Thread.ID, &stderr)
	if err != nil {
		return &CodexRunResult{
			SessionID:         start.Thread.ID,
			ThreadURL:         CodexThreadURL(start.Thread.ID),
			ThreadURLStatus:   "created as a Codex Desktop thread",
			OutputLastMessage: opts.OutputLastMessage,
			CombinedOutput:    stderr.String(),
		}, err
	}
	if opts.OutputLastMessage != "" {
		if err := writeOutputLastMessage(opts.OutputLastMessage, lastMessage); err != nil {
			return nil, err
		}
	}
	return &CodexRunResult{
		SessionID:         start.Thread.ID,
		ThreadURL:         CodexThreadURL(start.Thread.ID),
		ThreadURLStatus:   "created as a Codex Desktop thread",
		PickupCommand:     "open " + CodexThreadURL(start.Thread.ID),
		OutputLastMessage: opts.OutputLastMessage,
		CombinedOutput:    lastMessage,
	}, nil
}

func readAppServerMessages(reader io.Reader, messages chan<- appServerMessage, readErrs chan<- error) {
	defer close(messages)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		msg := appServerMessage{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			readErrs <- err
			return
		}
		messages <- msg
	}
	if err := scanner.Err(); err != nil {
		readErrs <- err
	}
}

func waitForResponse(ctx context.Context, messages <-chan appServerMessage, readErrs <-chan error, id int, stderr *bytes.Buffer) (appServerMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return appServerMessage{}, ctx.Err()
		case err := <-readErrs:
			if err != nil {
				return appServerMessage{}, fmt.Errorf("read app-server response: %w: %s", err, tail(stderr.String(), 20))
			}
		case msg, ok := <-messages:
			if !ok {
				return appServerMessage{}, fmt.Errorf("app-server exited before response %d: %s", id, tail(stderr.String(), 20))
			}
			if msg.ID != id {
				continue
			}
			if len(msg.Error) > 0 {
				return appServerMessage{}, fmt.Errorf("app-server %s failed: %s", msg.Method, string(msg.Error))
			}
			return msg, nil
		}
	}
}

func waitForTurnCompleted(ctx context.Context, messages <-chan appServerMessage, readErrs <-chan error, threadID string, stderr *bytes.Buffer) (string, error) {
	var builder strings.Builder
	lastCompleted := ""
	for {
		select {
		case <-ctx.Done():
			return strings.TrimSpace(builder.String()), ctx.Err()
		case err := <-readErrs:
			if err != nil {
				return strings.TrimSpace(builder.String()), fmt.Errorf("read app-server notification: %w: %s", err, tail(stderr.String(), 20))
			}
		case msg, ok := <-messages:
			if !ok {
				return strings.TrimSpace(builder.String()), fmt.Errorf("app-server exited before turn completed: %s", tail(stderr.String(), 20))
			}
			switch msg.Method {
			case "item/agentMessage/delta":
				params := appServerAgentDeltaParams{}
				if json.Unmarshal(msg.Params, &params) == nil && params.ThreadID == threadID {
					builder.WriteString(params.Delta)
				}
			case "item/completed":
				params := appServerItemCompletedParams{}
				if json.Unmarshal(msg.Params, &params) == nil && params.ThreadID == threadID && params.Item.Type == "agentMessage" {
					lastCompleted = params.Item.Text
				}
			case "turn/completed":
				params := appServerTurnCompletedParams{}
				if json.Unmarshal(msg.Params, &params) != nil || params.ThreadID != threadID {
					continue
				}
				if params.Turn.Status != "completed" {
					message := params.Turn.Status
					if params.Turn.Error != nil && params.Turn.Error.Message != "" {
						message = params.Turn.Error.Message
					}
					return strings.TrimSpace(builder.String()), fmt.Errorf("turn failed: %s", message)
				}
				if lastCompleted != "" {
					return strings.TrimSpace(lastCompleted), nil
				}
				return strings.TrimSpace(builder.String()), nil
			}
		}
	}
}

func writeOutputLastMessage(path string, message string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(message), 0o644)
}
