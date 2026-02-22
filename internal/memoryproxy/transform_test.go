package memoryproxy

import (
	"encoding/json"
	"testing"
)

func TestTransformOpenAIChatCompletionsPrependsSystemHint(t *testing.T) {
	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"Use npm"}]}`)
	out, res := Transform("openai", "/v1/chat/completions", body, "Use bun by default")
	if !res.Injectable || !res.Mutated {
		t.Fatalf("expected mutated injectable result, got %+v", res)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal transformed payload: %v", err)
	}
	msgs, ok := payload["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected messages array with prepended system message")
	}
	first, _ := msgs[0].(map[string]any)
	if role, _ := first["role"].(string); role != "system" {
		t.Fatalf("expected first role=system, got %v", first)
	}
}

func TestTransformAnthropicAddsSystemHint(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet","messages":[{"role":"user","content":"Use pip"}]}`)
	out, res := Transform("anthropic", "/v1/messages", body, "Use uv by default")
	if !res.Injectable || !res.Mutated {
		t.Fatalf("expected mutated injectable result, got %+v", res)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal transformed payload: %v", err)
	}
	system, _ := payload["system"].(string)
	if system == "" {
		t.Fatalf("expected system hint to be injected")
	}
}

func TestTransformRejectsNonJSONAsEncryptedOrNonJSON(t *testing.T) {
	_, res := Transform("openai", "/v1/chat/completions", []byte("\x00\x01\x02"), "hint")
	if res.Injectable {
		t.Fatalf("expected non-injectable for non-json payload")
	}
	if res.Reason == "" {
		t.Fatalf("expected reason")
	}
}
