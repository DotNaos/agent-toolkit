package memoryd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent-toolkit/internal/memoryproxy"
)

type Server struct {
	listenAddr string
	store      *Store
	retriever  *Retriever
	v2         *V2Service
	httpServer *http.Server
}

func NewServer(cfg ServerConfig) (*Server, error) {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return nil, fmt.Errorf("db path is required")
	}
	store, err := NewStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	var embedder Embedder
	if strings.TrimSpace(cfg.OllamaURL) != "-" {
		embedder = NewOllamaEmbedder(cfg.OllamaURL, cfg.EmbeddingModel)
	}
	retriever := NewRetriever(store, embedder, cfg.ScoreThreshold)

	s := &Server{listenAddr: cfg.ListenAddr, store: store, retriever: retriever}
	s.v2 = NewV2ServiceWithRoot(store, retriever, cfg.MemoryReposRoot)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/memory/upsert", s.handleMemoryUpsert)
	mux.HandleFunc("/v1/memory/search", s.handleMemorySearch)
	mux.HandleFunc("/v1/memory/list", s.handleMemoryList)
	mux.HandleFunc("/v1/memory/delete", s.handleMemoryDelete)
	mux.HandleFunc("/v1/memory/seed", s.handleMemorySeed)
	mux.HandleFunc("/v1/repo/sync", s.handleRepoSync)
	mux.HandleFunc("/v1/proxy/transform", s.handleProxyTransform)
	mux.HandleFunc("/v1/proxy/event", s.handleProxyEvent)
	mux.HandleFunc("/v1/compat/report", s.handleCompatReport)
	s.registerV2Routes(mux)

	s.httpServer = &http.Server{Addr: cfg.ListenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s, nil
}

func (s *Server) Run() error {
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Close(ctx context.Context) error {
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(ctx)
	}
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "health", "listen": s.listenAddr})
}

func (s *Server) handleMemoryUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		ID         string   `json:"id"`
		Scope      string   `json:"scope"`
		RepoPath   string   `json:"repo_path"`
		Category   string   `json:"category"`
		Title      string   `json:"title"`
		Content    string   `json:"content"`
		Language   string   `json:"language"`
		Tags       []string `json:"tags"`
		SourceType string   `json:"source_type"`
		Active     *bool    `json:"active"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	mem, embedded, err := s.retriever.UpsertWithEmbedding(r.Context(), UpsertMemoryParams{
		ID:         req.ID,
		Scope:      MemoryScope(req.Scope),
		RepoPath:   cleanMaybePath(req.RepoPath),
		Category:   MemoryCategory(req.Category),
		Title:      req.Title,
		Content:    req.Content,
		Language:   req.Language,
		Tags:       req.Tags,
		SourceType: MemorySourceType(req.SourceType),
		Active:     active,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.upsert", "memory": mem, "embedding_saved": embedded})
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Query          string   `json:"query"`
		RepoPath       string   `json:"repo_path"`
		Categories     []string `json:"categories"`
		Limit          int      `json:"limit"`
		ScoreThreshold float64  `json:"score_threshold"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cats := make([]MemoryCategory, 0, len(req.Categories))
	for _, c := range req.Categories {
		cats = append(cats, MemoryCategory(c))
	}
	resp, err := s.retriever.Search(r.Context(), SearchParams{Query: req.Query, RepoPath: req.RepoPath, Categories: cats, Limit: req.Limit, ScoreThreshold: req.ScoreThreshold})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.search", "search": resp})
}

func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	items, err := s.store.ListMemories(r.URL.Query().Get("scope"), r.URL.Query().Get("repo_path"), parseIntDefault(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.list", "memories": items})
}

func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteMemory(req.ID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.delete", "id": req.ID})
}

func (s *Server) handleMemorySeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	seed := []UpsertMemoryParams{
		{Scope: ScopeGlobal, Category: CategoryTooling, Title: "Frontend package/runtime preference", Content: "Use bun by default for frontend work unless the repo explicitly uses another package manager/runtime.", Language: "en", Tags: []string{"bun", "frontend", "tooling"}, SourceType: SourceSeed, Active: true},
		{Scope: ScopeGlobal, Category: CategoryTooling, Title: "Python tooling preference", Content: "Use uv by default for Python environments, dependencies and task execution unless the repo explicitly uses something else.", Language: "en", Tags: []string{"uv", "python", "tooling"}, SourceType: SourceSeed, Active: true},
	}
	added := 0
	for _, p := range seed {
		if _, _, err := s.retriever.UpsertWithEmbedding(r.Context(), p); err == nil {
			added++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.seed", "added": added})
}

func (s *Server) handleRepoSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := SyncRepoPreferences(s.store, req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "repo.sync", "result": res})
}

func (s *Server) handleProxyTransform(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req ProxyTransformRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	provider := strings.TrimSpace(strings.ToLower(req.Provider))
	if provider == "" {
		provider = memoryproxy.ProviderFromHost(req.Host)
	}
	bodyBytes, err := base64.StdEncoding.DecodeString(req.BodyBase64)
	if err != nil {
		writeJSON(w, http.StatusOK, ProxyTransformResponse{Status: "success", Injectable: false, Mutated: false, Provider: provider, Reason: "invalid_base64"})
		return
	}
	if isCompressed(req.Headers) {
		writeJSON(w, http.StatusOK, ProxyTransformResponse{Status: "success", Injectable: false, Mutated: false, Provider: provider, BodyBase64: req.BodyBase64, Reason: "payload_encrypted_or_compressed"})
		return
	}

	queryText := extractQueryTextFromJSON(bodyBytes)
	repoPath := extractRepoPath(bodyBytes)
	searchResp, searchErr := s.retriever.Search(r.Context(), SearchParams{Query: queryText, RepoPath: repoPath, Limit: DefaultProxyHintsLimit})
	if searchErr != nil {
		writeJSON(w, http.StatusOK, ProxyTransformResponse{Status: "success", Injectable: false, Mutated: false, Provider: provider, BodyBase64: req.BodyBase64, Reason: "search_failed"})
		return
	}
	hint := BuildAgentHint(searchResp.Results)
	outBody, tr := memoryproxy.Transform(provider, req.Path, bodyBytes, hint)
	outB64 := req.BodyBase64
	if tr.Mutated {
		outB64 = base64.StdEncoding.EncodeToString(outBody)
	}
	writeJSON(w, http.StatusOK, ProxyTransformResponse{Status: "success", Injectable: tr.Injectable, Mutated: tr.Mutated, Provider: provider, BodyBase64: outB64, Hint: hintIfMutated(tr.Mutated, hint), Reason: tr.Reason, FallbackUsed: searchResp.FallbackUsed})
}

func (s *Server) handleProxyEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var evt ProxyEvent
	if err := decodeJSON(r, &evt); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	evt.ErrorRedacted = RedactSecrets(evt.ErrorRedacted)
	if err := s.store.LogProxyEvent(evt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "proxy.event"})
}

func (s *Server) handleCompatReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	rows, err := s.store.CompatReport(parseIntDefault(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "compat.report", "items": rows})
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, map[string]any{"status": "error", "message": err.Error()})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func parseIntDefault(raw string, def int) int {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	var v int
	_, err := fmt.Sscanf(raw, "%d", &v)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func isCompressed(headers map[string]string) bool {
	if len(headers) == 0 {
		return false
	}
	for k, v := range headers {
		if strings.EqualFold(k, "content-encoding") && strings.TrimSpace(v) != "" && !strings.EqualFold(strings.TrimSpace(v), "identity") {
			return true
		}
	}
	return false
}

func hintIfMutated(mutated bool, hint string) string {
	if !mutated {
		return ""
	}
	return hint
}

var pathRegex = regexp.MustCompile(`(?:/Users|/home)/[^\s\"']+`)

func extractRepoPath(body []byte) string {
	matches := pathRegex.FindAllString(string(body), -1)
	for _, m := range matches {
		p := filepath.Clean(m)
		if root := findRepoRoot(p); root != "" {
			return root
		}
		if strings.Contains(p, "/") {
			return p
		}
	}
	return ""
}

func findRepoRoot(path string) string {
	cur := path
	if info, err := os.Stat(cur); err == nil && !info.IsDir() {
		cur = filepath.Dir(cur)
	}
	for {
		if cur == "" || cur == "/" || cur == "." {
			return ""
		}
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

func extractQueryTextFromJSON(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	buf := make([]string, 0, 32)
	collectStrings(v, &buf)
	joined := strings.Join(buf, "\n")
	if len(joined) > 8000 {
		joined = joined[:8000]
	}
	return joined
}

func collectStrings(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		t := strings.TrimSpace(x)
		if t != "" {
			*out = append(*out, t)
		}
	case []any:
		for _, item := range x {
			collectStrings(item, out)
		}
	case map[string]any:
		keys := []string{"instructions", "system", "content", "text", "input", "prompt"}
		for _, k := range keys {
			if val, ok := x[k]; ok {
				collectStrings(val, out)
			}
		}
		for _, val := range x {
			collectStrings(val, out)
		}
	}
}
