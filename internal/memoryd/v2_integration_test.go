package memoryd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireJJ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed")
	}
}

func TestEpisodeSnapshotMarkdownRoundtrip(t *testing.T) {
	ep := EpisodeDocument{
		Frontmatter: EpisodeFrontmatter{EpisodeID: "e1", CreatedAt: "2026-01-01T00:00:00Z", RepoID: "r1", RepoPath: "/tmp/repo", Targets: []string{"topic/tooling"}, Source: EpisodeSourceManual, Kind: EpisodeKindManualNote},
		Sections:    EpisodeSections{StepSummary: "Summary", Facts: []string{"Fact A"}, Decisions: []string{"Use bun"}},
	}
	raw := renderEpisodeMarkdown(ep)
	parsed, err := parseEpisodeMarkdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Frontmatter.EpisodeID != ep.Frontmatter.EpisodeID {
		t.Fatalf("episode frontmatter mismatch")
	}
	if len(parsed.Sections.Decisions) != 1 || parsed.Sections.Decisions[0] != "Use bun" {
		t.Fatalf("episode decisions mismatch: %+v", parsed.Sections.Decisions)
	}

	snap := SnapshotDocument{Frontmatter: SnapshotFrontmatter{SnapshotID: "s1", LogicalID: "mem/topic/tooling", Revision: 1, RepoID: "r1", Target: "topic/tooling", GeneratedAt: "2026-01-01T00:00:00Z", ConflictPolicy: "newest-wins+mark"}, Sections: SnapshotSections{Facts: []string{"F1"}, Decisions: []string{"D1"}}}
	rawS := renderSnapshotMarkdown(snap)
	parsedS, err := parseSnapshotMarkdown(rawS)
	if err != nil {
		t.Fatal(err)
	}
	if parsedS.Frontmatter.SnapshotID != "s1" || parsedS.Frontmatter.Revision != 1 {
		t.Fatalf("snapshot frontmatter mismatch")
	}
}

func TestV2TaskEpisodeConsolidateAndProxyTransform(t *testing.T) {
	requireJJ(t)
	projectRepo := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{DBPath: filepath.Join(t.TempDir(), "memory.db"), OllamaURL: "-", MemoryReposRoot: filepath.Join(t.TempDir(), "memory-repos")})
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := httptest.NewServer(server.httpServer.Handler)
	defer func() {
		httpSrv.Close()
		_ = server.Close(context.Background())
	}()

	post := func(path string, payload any) map[string]any {
		b, _ := json.Marshal(payload)
		resp, err := http.Post(httpSrv.URL+path, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%v", resp.StatusCode, out)
		}
		return out
	}

	start := post("/v2/memory/task/start", map[string]any{"repo_path": projectRepo})
	taskMap, _ := start["task"].(map[string]any)
	taskID, _ := taskMap["task_id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %v", start)
	}

	_ = post("/v2/memory/episode/create", map[string]any{
		"repo_path":    projectRepo,
		"task_id":      taskID,
		"targets":      []string{"topic/tooling"},
		"source":       "manual",
		"kind":         "manual-note",
		"step_summary": "Document tooling defaults",
		"facts":        []string{"Frontend runtime: bun"},
		"decisions":    []string{"Use bun by default for frontend work."},
		"interfaces":   []string{"Package manager command interface: bun run"},
	})

	end := post("/v2/memory/task/end", map[string]any{"repo_path": projectRepo, "task_id": taskID})
	res, _ := end["result"].(map[string]any)
	gen, _ := res["generated"].([]any)
	if len(gen) == 0 {
		t.Fatalf("expected generated snapshots, got %v", end)
	}

	resolve := post("/v2/memory/snapshot/resolve", map[string]any{"repo_path": projectRepo, "targets": []string{"topic/tooling"}})
	snaps, _ := resolve["snapshots"].([]any)
	if len(snaps) == 0 {
		t.Fatalf("expected resolved snapshot, got %v", resolve)
	}

	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"Implement frontend feature and use npm"}]}`)
	proxy := post("/v2/proxy/transform", map[string]any{
		"provider":  "openai",
		"host":      "api.openai.com",
		"path":      "/v1/chat/completions",
		"repo_path": projectRepo,
		"body_b64":  base64.StdEncoding.EncodeToString(body),
	})
	if mutated, _ := proxy["mutated"].(bool); !mutated {
		t.Fatalf("expected v2 proxy transform to mutate request, got %v", proxy)
	}
	if _, ok := proxy["resolved_refs"]; !ok {
		t.Fatalf("expected resolved_refs in response: %v", proxy)
	}
}
