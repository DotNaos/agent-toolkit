package chatd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testServer struct {
	httpURL string
	close   func()
}

func newTestServer(t *testing.T, dbPath string) *testServer {
	t.Helper()
	store, err := NewSQLiteStore(dbPath, DefaultLeaseDuration)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	srv, err := NewServer(ServerConfig{
		ListenAddr:   DefaultListenAddr,
		DBPath:       dbPath,
		Store:        store,
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	httpSrv := httptest.NewServer(srv.httpServer.Handler)

	return &testServer{
		httpURL: httpSrv.URL,
		close: func() {
			httpSrv.Close()
			_ = srv.Close()
		},
	}
}

func postJSONRequest(t *testing.T, baseURL, endpoint string, payload any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	resp, err := http.Post(baseURL+endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post request failed: %v", err)
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	return resp.StatusCode, out
}

func TestServer_SendWaitAck(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat.db")
	ts := newTestServer(t, dbPath)
	defer ts.close()

	status, sendResp := postJSONRequest(t, ts.httpURL, "/v1/messages/send", map[string]any{
		"to_agent":   "agent-a",
		"from_agent": "agent-b",
		"thread_id":  "thread-1",
		"body":       "hello",
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on send, got %d (%v)", status, sendResp)
	}

	messageID, _ := sendResp["message_id"].(string)
	if messageID == "" {
		t.Fatalf("expected message_id in send response: %v", sendResp)
	}

	status, waitResp := postJSONRequest(t, ts.httpURL, "/v1/messages/wait", map[string]any{
		"agent":   "agent-a",
		"timeout": "1s",
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on wait, got %d (%v)", status, waitResp)
	}
	if waitResp["status"] != "success" {
		t.Fatalf("expected wait success, got %v", waitResp)
	}

	leaseToken, _ := waitResp["lease_token"].(string)
	if leaseToken == "" {
		t.Fatalf("expected lease_token in wait response: %v", waitResp)
	}

	messageObj, _ := waitResp["message"].(map[string]any)
	if gotID, _ := messageObj["id"].(string); gotID != messageID {
		t.Fatalf("expected leased id %s, got %s", messageID, gotID)
	}

	status, ackResp := postJSONRequest(t, ts.httpURL, "/v1/messages/ack", map[string]any{
		"agent":       "agent-a",
		"message_id":  messageID,
		"lease_token": leaseToken,
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on ack, got %d (%v)", status, ackResp)
	}
	if ackResp["status"] != "success" {
		t.Fatalf("expected ack success, got %v", ackResp)
	}
}

func TestServer_WaitTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat.db")
	ts := newTestServer(t, dbPath)
	defer ts.close()

	status, waitResp := postJSONRequest(t, ts.httpURL, "/v1/messages/wait", map[string]any{
		"agent":   "agent-a",
		"timeout": "1s",
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on wait timeout, got %d (%v)", status, waitResp)
	}
	if waitResp["status"] != "timeout" {
		t.Fatalf("expected timeout status, got %v", waitResp)
	}
}

func TestServer_AckRejectsWrongLeaseToken(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat.db")
	ts := newTestServer(t, dbPath)
	defer ts.close()

	_, sendResp := postJSONRequest(t, ts.httpURL, "/v1/messages/send", map[string]any{
		"to_agent": "agent-a",
		"body":     "hello",
	})
	messageID, _ := sendResp["message_id"].(string)

	_, waitResp := postJSONRequest(t, ts.httpURL, "/v1/messages/wait", map[string]any{
		"agent":   "agent-a",
		"timeout": "1s",
	})
	_ = waitResp

	status, ackResp := postJSONRequest(t, ts.httpURL, "/v1/messages/ack", map[string]any{
		"agent":       "agent-a",
		"message_id":  messageID,
		"lease_token": "wrong-token",
	})
	if status != http.StatusConflict {
		t.Fatalf("expected 409 on wrong lease token, got %d (%v)", status, ackResp)
	}
}

func TestServer_ParallelWaitersReceiveDifferentMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat.db")
	ts := newTestServer(t, dbPath)
	defer ts.close()

	_, _ = postJSONRequest(t, ts.httpURL, "/v1/messages/send", map[string]any{"to_agent": "agent-a", "body": "m1"})
	_, _ = postJSONRequest(t, ts.httpURL, "/v1/messages/send", map[string]any{"to_agent": "agent-a", "body": "m2"})

	ids := make(chan string, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			status, resp := postJSONRequest(t, ts.httpURL, "/v1/messages/wait", map[string]any{
				"agent":   "agent-a",
				"timeout": "2s",
			})
			if status != http.StatusOK || resp["status"] != "success" {
				t.Errorf("wait failed: status=%d resp=%v", status, resp)
				return
			}
			msg, _ := resp["message"].(map[string]any)
			id, _ := msg["id"].(string)
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)

	seen := map[string]bool{}
	for id := range ids {
		if id == "" {
			t.Fatal("received empty message id")
		}
		if seen[id] {
			t.Fatalf("duplicate message id delivered to parallel waiters: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 distinct messages, got %d", len(seen))
	}
}

func TestServer_RestartPreservesUnackedMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat.db")

	ts1 := newTestServer(t, dbPath)
	_, _ = postJSONRequest(t, ts1.httpURL, "/v1/messages/send", map[string]any{
		"to_agent": "agent-a",
		"body":     "persist-me",
	})
	ts1.close()

	ts2 := newTestServer(t, dbPath)
	defer ts2.close()

	status, resp := postJSONRequest(t, ts2.httpURL, "/v1/messages/wait", map[string]any{
		"agent":   "agent-a",
		"timeout": "1s",
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 on wait after restart, got %d (%v)", status, resp)
	}
	if resp["status"] != "success" {
		t.Fatalf("expected success after restart, got %v", resp)
	}

	msg, _ := resp["message"].(map[string]any)
	if body, _ := msg["body"].(string); body != "persist-me" {
		t.Fatalf("expected persisted message body, got %q", body)
	}
}
