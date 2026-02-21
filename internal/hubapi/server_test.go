package hubapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("failed to write index file: %v", err)
	}

	s, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		DBPath:     filepath.Join(t.TempDir(), "hub.db"),
		WebDir:     webDir,
	})
	if err != nil {
		t.Fatalf("new server failed: %v", err)
	}

	ts := httptest.NewServer(s.httpServer.Handler)
	t.Cleanup(func() {
		ts.Close()
		_ = s.Close(context.Background())
	})
	return s, ts
}

func postJSON(t *testing.T, client *http.Client, url string, payload any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	return resp.StatusCode, out
}

func TestConversationMessageAndDispatchFlow(t *testing.T) {
	_, ts := newTestServer(t)
	client := ts.Client()

	status, conv := postJSON(t, client, ts.URL+"/v1/conversations", map[string]any{
		"name":         "Main",
		"participants": []map[string]any{{"type": "human", "id": "owner"}, {"type": "agent", "id": "agent-a"}},
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (%v)", status, conv)
	}
	conversationID, _ := conv["conversation_id"].(string)

	status, msg := postJSON(t, client, ts.URL+"/v1/messages", map[string]any{
		"conversation_id": conversationID,
		"from_id":         "owner",
		"body":            "hello",
		"kind":            "text",
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on message, got %d (%v)", status, msg)
	}

	resp, err := client.Get(ts.URL + "/v1/messages?conversation_id=" + conversationID)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	defer resp.Body.Close()
	var list map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode messages failed: %v", err)
	}
	messages, _ := list["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("expected at least one message")
	}

	status, dispatch := postJSON(t, client, ts.URL+"/v1/agents/agent-a/dispatch", map[string]any{
		"conversation_id": conversationID,
		"prompt":          "deploy to production",
		"metadata":        map[string]any{"action": "deploy"},
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on dispatch, got %d (%v)", status, dispatch)
	}
	if got, _ := dispatch["status"].(string); got != "waiting_approval" {
		t.Fatalf("expected waiting_approval status, got %q", got)
	}

	resp, err = client.Get(ts.URL + "/v1/approvals/pending?conversation_id=" + conversationID)
	if err != nil {
		t.Fatalf("list approvals failed: %v", err)
	}
	defer resp.Body.Close()
	var approvals map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&approvals); err != nil {
		t.Fatalf("decode approvals failed: %v", err)
	}
	items, _ := approvals["approvals"].([]any)
	if len(items) == 0 {
		t.Fatal("expected pending approval")
	}

	first := items[0].(map[string]any)
	approvalID, _ := first["id"].(string)
	status, approveResp := postJSON(t, client, ts.URL+"/v1/approvals/"+approvalID+"/respond", map[string]any{
		"human_id": "owner",
		"decision": "approve",
		"payload":  map[string]any{"decision": "approve"},
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on approval response, got %d (%v)", status, approveResp)
	}
}

func TestSSEReceivesMessageCreatedEvent(t *testing.T) {
	_, ts := newTestServer(t)
	client := ts.Client()

	_, conv := postJSON(t, client, ts.URL+"/v1/conversations", map[string]any{
		"name":         "SSE",
		"participants": []map[string]any{{"type": "human", "id": "owner"}, {"type": "agent", "id": "agent-a"}},
	})
	conversationID, _ := conv["conversation_id"].(string)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/events/stream?conversation_id="+conversationID, nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("sse connect failed: %v", err)
	}
	defer resp.Body.Close()

	_, _ = postJSON(t, client, ts.URL+"/v1/messages", map[string]any{
		"conversation_id": conversationID,
		"from_id":         "owner",
		"body":            "hello sse",
		"kind":            "text",
	})

	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "event: message.created") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected message.created event in SSE stream")
	}
}
