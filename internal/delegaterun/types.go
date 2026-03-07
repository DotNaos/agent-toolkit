package delegaterun

import (
	"fmt"
	"strings"
)

type Mode string

const (
	ModeAdvisory         Mode = "advisory"
	ModeGuardedExecution Mode = "guarded_execution"
)

type Status string

const (
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusBlocked   Status = "blocked"
	StatusTimedOut  Status = "timed_out"
)

type ContextItem struct {
	Type      string `json:"type,omitempty"`
	Label     string `json:"label,omitempty"`
	Text      string `json:"text,omitempty"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type Request struct {
	Adapter    string         `json:"adapter"`
	Model      string         `json:"model,omitempty"`
	Task       string         `json:"task"`
	Mode       Mode           `json:"mode"`
	CWD        string         `json:"cwd,omitempty"`
	Context    []ContextItem  `json:"context,omitempty"`
	TimeoutSec int            `json:"timeout_sec,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Risk struct {
	ApprovalRequired bool   `json:"approval_required"`
	Reason           string `json:"reason,omitempty"`
}

type Artifact struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Content string `json:"content,omitempty"`
}

type Result struct {
	Status     Status     `json:"status"`
	Adapter    string     `json:"adapter"`
	Mode       Mode       `json:"mode"`
	FinalText  string     `json:"final_text,omitempty"`
	Stdout     string     `json:"stdout,omitempty"`
	Stderr     string     `json:"stderr,omitempty"`
	ExitCode   int        `json:"exit_code,omitempty"`
	DurationMS int64      `json:"duration_ms"`
	Artifacts  []Artifact `json:"artifacts,omitempty"`
	Risk       Risk       `json:"risk"`
}

type RunOptions struct {
	ApprovalGranted bool
}

func (r *Request) Normalize() {
	r.Adapter = strings.TrimSpace(strings.ToLower(r.Adapter))
	r.Model = strings.TrimSpace(r.Model)
	r.Task = strings.TrimSpace(r.Task)
	if r.Mode == "" {
		r.Mode = ModeAdvisory
	}
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
}

func (r Request) Validate() error {
	switch r.Adapter {
	case "gemini", "claude", "copilot", "codex":
	default:
		return fmt.Errorf("unsupported adapter %q", r.Adapter)
	}
	if strings.TrimSpace(r.Task) == "" {
		return fmt.Errorf("task is required")
	}
	switch r.Mode {
	case ModeAdvisory, ModeGuardedExecution:
	default:
		return fmt.Errorf("invalid mode %q", r.Mode)
	}
	return nil
}
