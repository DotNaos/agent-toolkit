package handovercli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildBriefingIncludesHandoverFields(t *testing.T) {
	briefing := BuildBriefing(BriefingOptions{
		Title:           "Project Goals",
		SourceSession:   "019e381e-38c9-72c0-9e91-9e59c8c51bf8",
		SourceProject:   "/source",
		TargetProject:   "/target",
		Mode:            "worktree",
		Idea:            "Use GOALS.md as project state.",
		RequestedChange: "Add it to the template.",
		Acceptance:      []string{"Generated projects include GOALS.md."},
		CreatedAt:       time.Date(2026, 5, 18, 1, 2, 3, 0, time.UTC),
	})

	for _, want := range []string{
		"kind: codex-handover",
		"source_session: \"019e381e-38c9-72c0-9e91-9e59c8c51bf8\"",
		"target_project: \"/target\"",
		"Hi Codex,",
		"Use GOALS.md as project state.",
		"Generated projects include GOALS.md.",
	} {
		if !strings.Contains(briefing, want) {
			t.Fatalf("briefing missing %q:\n%s", want, briefing)
		}
	}
}

func TestParseSessionID(t *testing.T) {
	output := "noise\nsession id: 019e381e-38c9-72c0-9e91-9e59c8c51bf8\nmore"
	got := ParseSessionID(output)
	if got != "019e381e-38c9-72c0-9e91-9e59c8c51bf8" {
		t.Fatalf("ParseSessionID() = %q", got)
	}
}

func TestRunCodexExecReturnsDeepLinkAndCLIPickup(t *testing.T) {
	fakeCodex := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\nprintf 'session id: 019e381e-38c9-72c0-9e91-9e59c8c51bf8\\n'\n"
	if err := os.WriteFile(fakeCodex, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := RunCodexExec(context.Background(), CodexRunOptions{
		CodexBin:      fakeCodex,
		TargetProject: t.TempDir(),
		Sandbox:       "read-only",
		Briefing:      "handover",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadURL != "codex://threads/019e381e-38c9-72c0-9e91-9e59c8c51bf8" {
		t.Fatalf("ThreadURL = %q, want Codex deep link", result.ThreadURL)
	}
	if !strings.Contains(result.ThreadURLStatus, "unsupported") {
		t.Fatalf("ThreadURLStatus = %q, want unsupported warning", result.ThreadURLStatus)
	}
	if !strings.Contains(result.PickupCommand, "exec resume 019e381e-38c9-72c0-9e91-9e59c8c51bf8") {
		t.Fatalf("PickupCommand = %q, want resume command", result.PickupCommand)
	}
}
