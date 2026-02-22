package memoryproxy

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Result struct {
	Injectable bool
	Mutated    bool
	Reason     string
}

func Transform(provider, path string, body []byte, hint string) ([]byte, Result) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.TrimSpace(hint) == "" {
		return body, Result{Injectable: true, Mutated: false, Reason: "no_relevant_hint"}
	}
	switch provider {
	case "openai":
		return transformOpenAI(path, body, hint)
	case "anthropic":
		return transformAnthropic(path, body, hint)
	default:
		return body, Result{Injectable: false, Mutated: false, Reason: "unsupported_provider"}
	}
}

func transformOpenAI(path string, body []byte, hint string) ([]byte, Result) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return body, Result{Injectable: false, Mutated: false, Reason: "payload_encrypted_or_non_json"}
	}
	mutated := false

	// Responses API: append to instructions when present/possible.
	if strings.Contains(path, "/responses") {
		if v, ok := raw["instructions"].(string); ok {
			raw["instructions"] = appendHintText(v, hint)
			mutated = true
		} else if _, exists := raw["instructions"]; !exists {
			raw["instructions"] = hint
			mutated = true
		}
		if !mutated {
			// Fall back to top-level input wrapper only if string input.
			if s, ok := raw["input"].(string); ok {
				raw["input"] = hint + "\n\n" + s
				mutated = true
			}
		}
	}

	// Chat Completions API: prepend system message.
	if !mutated {
		if msgs, ok := raw["messages"].([]any); ok {
			sysMsg := map[string]any{"role": "system", "content": hint}
			raw["messages"] = append([]any{sysMsg}, msgs...)
			mutated = true
		}
	}

	if !mutated {
		return body, Result{Injectable: false, Mutated: false, Reason: "unsupported_openai_shape"}
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return body, Result{Injectable: false, Mutated: false, Reason: "marshal_failed"}
	}
	return out, Result{Injectable: true, Mutated: true, Reason: "hint_injected"}
}

func transformAnthropic(path string, body []byte, hint string) ([]byte, Result) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return body, Result{Injectable: false, Mutated: false, Reason: "payload_encrypted_or_non_json"}
	}
	if !strings.Contains(path, "/messages") && !strings.Contains(path, "/v1") {
		return body, Result{Injectable: false, Mutated: false, Reason: "unsupported_anthropic_route"}
	}
	mutated := false
	switch v := raw["system"].(type) {
	case string:
		raw["system"] = appendHintText(v, hint)
		mutated = true
	case nil:
		raw["system"] = hint
		mutated = true
	case []any:
		// Anthropic supports structured system blocks in some SDKs. Add a text block.
		block := map[string]any{"type": "text", "text": hint}
		raw["system"] = append([]any{block}, v...)
		mutated = true
	}
	if !mutated {
		return body, Result{Injectable: false, Mutated: false, Reason: "unsupported_anthropic_shape"}
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return body, Result{Injectable: false, Mutated: false, Reason: "marshal_failed"}
	}
	return out, Result{Injectable: true, Mutated: true, Reason: "hint_injected"}
}

func appendHintText(existing, hint string) string {
	existing = strings.TrimSpace(existing)
	hint = strings.TrimSpace(hint)
	if existing == "" {
		return hint
	}
	if strings.Contains(existing, hint) {
		return existing
	}
	return existing + "\n\n" + hint
}

func ProviderFromHost(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "openai"):
		return "openai"
	case strings.Contains(h, "anthropic"):
		return "anthropic"
	default:
		return ""
	}
}

func ValidateJSONBody(body []byte) error {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	return nil
}
