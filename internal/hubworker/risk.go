package hubworker

import (
	"strings"
)

var riskyKeywords = []string{
	"delete",
	"deploy",
	"publish",
	"write",
	"update",
	"edit",
	"remove",
	"execute",
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
}

func IsRiskyAction(prompt string, metadata map[string]any) (bool, string) {
	lowerPrompt := strings.ToLower(prompt)
	if action, ok := metadata["action"].(string); ok {
		a := strings.ToLower(strings.TrimSpace(action))
		if strings.Contains(a, "write") || strings.Contains(a, "deploy") || strings.Contains(a, "delete") || strings.Contains(a, "edit") {
			return true, "metadata action indicates side effects"
		}
		if strings.Contains(a, "read") || strings.Contains(a, "analyze") {
			return false, "metadata action indicates read-only"
		}
	}

	for _, keyword := range riskyKeywords {
		if strings.Contains(lowerPrompt, keyword) {
			return true, "prompt contains risky keyword: " + keyword
		}
	}

	hasSafe := false
	for _, keyword := range safeKeywords {
		if strings.Contains(lowerPrompt, keyword) {
			hasSafe = true
			break
		}
	}
	if hasSafe {
		return false, "prompt appears read/analyze only"
	}

	// Conservative default for unknown actions.
	return true, "unclassified action defaults to approval"
}
