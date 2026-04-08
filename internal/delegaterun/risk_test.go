package delegaterun

import "testing"

func TestAssessRiskAllowsUnclassifiedAdvisoryPrompt(t *testing.T) {
	risk := AssessRisk("Hello?", nil, ModeAdvisory)
	if risk.ApprovalRequired {
		t.Fatalf("expected advisory prompt to avoid approval, got %+v", risk)
	}
	if risk.Reason != "advisory mode defaults to no approval" {
		t.Fatalf("unexpected reason: %q", risk.Reason)
	}
}

func TestAssessRiskStillBlocksGuardedExecution(t *testing.T) {
	risk := AssessRisk("Hello?", nil, ModeGuardedExecution)
	if !risk.ApprovalRequired {
		t.Fatalf("expected guarded execution to require approval, got %+v", risk)
	}
}

func TestAssessRiskAllowsLocalWriteActionMetadata(t *testing.T) {
	risk := AssessRisk("Update note.txt", map[string]any{"action": "write"}, ModeAdvisory)
	if risk.ApprovalRequired {
		t.Fatalf("expected local write metadata to avoid approval, got %+v", risk)
	}
}

func TestAssessRequestRiskRequiresApprovalForWriteCapability(t *testing.T) {
	risk := AssessRequestRisk(Request{Task: "Update note.txt", Capabilities: []string{"write"}}, nil)
	if !risk.ApprovalRequired {
		t.Fatalf("expected write capability to require approval, got %+v", risk)
	}
}

func TestAssessRequestRiskDefaultsToReadCapability(t *testing.T) {
	risk := AssessRequestRisk(Request{Task: "Summarize the repo"}, nil)
	if risk.ApprovalRequired {
		t.Fatalf("expected default read capability to avoid approval, got %+v", risk)
	}
}
