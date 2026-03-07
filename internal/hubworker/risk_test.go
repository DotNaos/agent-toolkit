package hubworker

import "testing"

func TestIsRiskyAction_ReadOnly(t *testing.T) {
	risky, _ := IsRiskyAction("please read and summarize this file", map[string]any{"action": "read"})
	if risky {
		t.Fatal("expected read-only action to be non-risky")
	}
}

func TestIsRiskyAction_WriteKeyword(t *testing.T) {
	risky, _ := IsRiskyAction("write changes and deploy to prod", nil)
	if !risky {
		t.Fatal("expected write/deploy action to be risky")
	}
}

func TestIsRiskyAction_DefaultAdvisorySafe(t *testing.T) {
	risky, _ := IsRiskyAction("do the thing", nil)
	if risky {
		t.Fatal("expected unclassified advisory action to avoid approval")
	}
}
