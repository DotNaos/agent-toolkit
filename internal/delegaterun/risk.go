package delegaterun

import "strings"

var riskyKeywords = []string{
	"delete",
	"deploy",
	"publish",
	"run migration",
	"drop",
	"kill",
	"charge",
	"send email",
}

var safeKeywords = []string{
	"read",
	"analyze",
	"inspect",
	"list",
	"summarize",
	"explain",
	"review",
	"show",
	"design",
}

func AssessRisk(task string, metadata map[string]any, mode Mode) Risk {
	if mode == ModeGuardedExecution {
		return Risk{ApprovalRequired: true, Reason: "guarded_execution requires human approval"}
	}

	lowerTask := strings.ToLower(strings.TrimSpace(task))
	if action, ok := metadata["action"].(string); ok {
		lowerAction := strings.ToLower(strings.TrimSpace(action))
		if strings.Contains(lowerAction, "deploy") || strings.Contains(lowerAction, "delete") || strings.Contains(lowerAction, "publish") || strings.Contains(lowerAction, "migration") {
			return Risk{ApprovalRequired: true, Reason: "metadata action indicates side effects"}
		}
		if strings.Contains(lowerAction, "read") || strings.Contains(lowerAction, "analyze") {
			return Risk{ApprovalRequired: false, Reason: "metadata action indicates read-only"}
		}
	}

	for _, keyword := range riskyKeywords {
		if strings.Contains(lowerTask, keyword) {
			return Risk{ApprovalRequired: true, Reason: "task contains risky keyword: " + keyword}
		}
	}
	for _, keyword := range safeKeywords {
		if strings.Contains(lowerTask, keyword) {
			return Risk{ApprovalRequired: false, Reason: "task appears read-only"}
		}
	}

	return Risk{ApprovalRequired: false, Reason: "advisory mode defaults to no approval"}
}

func AssessRequestRisk(req Request, policy *PolicyConfig) Risk {
	req.Normalize()

	defaultCapabilities := []string{"read"}
	approvalRequiredFor := []string{"write", "exec", "network", "git"}
	allowHeuristicFallback := true
	if policy != nil {
		if len(policy.DefaultCapabilities) > 0 {
			defaultCapabilities = normalizeCapabilities(policy.DefaultCapabilities)
		}
		if len(policy.ApprovalRequiredFor) > 0 {
			approvalRequiredFor = normalizeCapabilities(policy.ApprovalRequiredFor)
		}
		allowHeuristicFallback = policy.AllowHeuristicFallback
	}

	requested := append([]string(nil), req.Capabilities...)
	if len(requested) == 0 {
		requested = append(requested, defaultCapabilities...)
	}
	if req.Mode == ModeGuardedExecution {
		for _, capability := range []string{"read", "write", "exec", "git"} {
			if !containsString(requested, capability) {
				requested = append(requested, capability)
			}
		}
	}

	for _, capability := range requested {
		if containsString(approvalRequiredFor, capability) {
			return Risk{ApprovalRequired: true, Reason: "requested capabilities require approval: " + capability}
		}
	}

	if allowHeuristicFallback {
		risk := AssessRisk(req.Task, req.Metadata, req.Mode)
		if risk.ApprovalRequired {
			return Risk{ApprovalRequired: true, Reason: "heuristic fallback: " + risk.Reason}
		}
	}

	return Risk{ApprovalRequired: false, Reason: "capabilities allowed without approval: " + strings.Join(requested, ", ")}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
