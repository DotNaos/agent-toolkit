package hubworker

import "agent-toolkit/internal/delegaterun"

func IsRiskyAction(prompt string, metadata map[string]any) (bool, string) {
	risk := delegaterun.AssessRisk(prompt, metadata, delegaterun.ModeAdvisory)
	return risk.ApprovalRequired, risk.Reason
}
