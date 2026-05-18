package handovercli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type CodexRunOptions struct {
	CodexBin          string
	TargetProject     string
	Sandbox           string
	Briefing          string
	OutputLastMessage string
	SkipGitRepoCheck  bool
}

type CodexRunResult struct {
	SessionID         string `json:"session_id,omitempty"`
	ThreadURL         string `json:"thread_url,omitempty"`
	ThreadURLStatus   string `json:"thread_url_status,omitempty"`
	PickupCommand     string `json:"pickup_command,omitempty"`
	OutputLastMessage string `json:"output_last_message,omitempty"`
	CombinedOutput    string `json:"combined_output,omitempty"`
}

var sessionIDPattern = regexp.MustCompile(`session id:\s*([0-9a-fA-F-]{36})`)

const execThreadURLUnsupported = "unsupported: codex exec sessions can be resumed by CLI, but Codex Desktop may stay on a loading screen"

func RunCodexExec(ctx context.Context, opts CodexRunOptions) (*CodexRunResult, error) {
	codexBin := firstNonEmpty(opts.CodexBin, "codex")
	sandbox := firstNonEmpty(opts.Sandbox, "read-only")
	if opts.TargetProject == "" {
		return nil, fmt.Errorf("--target-project is required")
	}
	if opts.Briefing == "" {
		return nil, fmt.Errorf("briefing is empty")
	}

	args := []string{"exec", "-C", opts.TargetProject, "-s", sandbox}
	if opts.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	if opts.OutputLastMessage != "" {
		args = append(args, "--output-last-message", opts.OutputLastMessage)
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, codexBin, args...)
	cmd.Stdin = strings.NewReader(opts.Briefing)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()

	text := output.String()
	result := &CodexRunResult{
		SessionID:         ParseSessionID(text),
		CombinedOutput:    text,
		OutputLastMessage: opts.OutputLastMessage,
	}
	if result.SessionID != "" {
		result.ThreadURL = CodexThreadURL(result.SessionID)
		result.ThreadURLStatus = execThreadURLUnsupported
		result.PickupCommand = CodexResumeCommand(codexBin, result.SessionID)
	}
	if err != nil {
		return result, fmt.Errorf("codex exec failed: %w: %s", err, tail(text, 30))
	}
	return result, nil
}

func RunCodexResume(ctx context.Context, codexBin string, sessionID string, prompt string, outputLastMessage string) (*CodexRunResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	codexBin = firstNonEmpty(codexBin, "codex")
	args := []string{"exec", "resume", sessionID}
	if outputLastMessage != "" {
		args = append(args, "--output-last-message", outputLastMessage)
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, codexBin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()

	text := output.String()
	result := &CodexRunResult{
		SessionID:         sessionID,
		ThreadURL:         CodexThreadURL(sessionID),
		ThreadURLStatus:   execThreadURLUnsupported,
		PickupCommand:     CodexResumeCommand(codexBin, sessionID),
		OutputLastMessage: outputLastMessage,
		CombinedOutput:    text,
	}
	if err != nil {
		return result, fmt.Errorf("codex resume failed: %w: %s", err, tail(text, 30))
	}
	return result, nil
}

func CodexThreadURL(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return "codex://threads/" + sessionID
}

func CodexResumeCommand(codexBin string, sessionID string) string {
	codexBin = firstNonEmpty(codexBin, "codex")
	if sessionID == "" {
		return ""
	}
	return fmt.Sprintf("%s exec resume %s -", codexBin, sessionID)
}

func ParseSessionID(output string) string {
	matches := sessionIDPattern.FindStringSubmatch(output)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func tail(text string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func timeoutContext(seconds int) (context.Context, context.CancelFunc) {
	if seconds <= 0 {
		seconds = 1800
	}
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}
