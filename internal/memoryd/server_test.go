package memoryd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestHTTPServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, err := NewServer(ServerConfig{DBPath: filepath.Join(t.TempDir(), "memory.db"), OllamaURL: "-"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	httpSrv := httptest.NewServer(srv.httpServer.Handler)
	t.Cleanup(func() {
		httpSrv.Close()
		_ = srv.Close(nil)
	})
	return srv, httpSrv
}

func postJSON(t *testing.T, base, path string, payload any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(payload)
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%v", resp.StatusCode, out)
	}
	return out
}

func TestServerProxyTransformInjectsHintForOpenAI(t *testing.T) {
	_, ts := newTestHTTPServer(t)
	_ = postJSON(t, ts.URL, "/v1/memory/upsert", map[string]any{
		"scope": "global", "category": "tooling", "title": "bun", "content": "Use bun by default for frontend work.", "source_type": "manual",
	})
	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"Please use npm for this frontend feature"}]}`)
	out := postJSON(t, ts.URL, "/v1/proxy/transform", map[string]any{
		"provider": "openai",
		"host":     "api.openai.com",
		"path":     "/v1/chat/completions",
		"body_b64": base64.StdEncoding.EncodeToString(body),
	})
	if mutated, _ := out["mutated"].(bool); !mutated {
		t.Fatalf("expected mutated=true, got %v", out)
	}
}

func TestServerProxyTransformCompressedPayloadFailOpen(t *testing.T) {
	_, ts := newTestHTTPServer(t)
	out := postJSON(t, ts.URL, "/v1/proxy/transform", map[string]any{
		"provider": "openai",
		"host":     "api.openai.com",
		"path":     "/v1/chat/completions",
		"headers":  map[string]string{"content-encoding": "gzip"},
		"body_b64": base64.StdEncoding.EncodeToString([]byte("not-json")),
	})
	if injectable, _ := out["injectable"].(bool); injectable {
		t.Fatalf("expected injectable=false, got %v", out)
	}
}
