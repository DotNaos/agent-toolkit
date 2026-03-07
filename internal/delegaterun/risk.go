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
