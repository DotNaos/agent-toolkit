package memoryd

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"agent-toolkit/internal/memoryproxy"
	"github.com/oklog/ulid/v2"
)

func (s *Server) registerV2Routes(mux *http.ServeMux) {
	mux.HandleFunc("/v2/memory/use-repo", s.handleV2UseRepo)
	mux.HandleFunc("/v2/memory/repos", s.handleV2Repos)
	mux.HandleFunc("/v2/memory/task/start", s.handleV2TaskStart)
	mux.HandleFunc("/v2/memory/task/end", s.handleV2TaskEnd)
	mux.HandleFunc("/v2/memory/episode/create", s.handleV2EpisodeCreate)
	mux.HandleFunc("/v2/memory/episode/abandon", s.handleV2EpisodeAbandon)
	mux.HandleFunc("/v2/memory/snapshot/list", s.handleV2SnapshotList)
	mux.HandleFunc("/v2/memory/snapshot/read", s.handleV2SnapshotRead)
	mux.HandleFunc("/v2/memory/snapshot/resolve", s.handleV2SnapshotResolve)
	mux.HandleFunc("/v2/memory/snapshot/consolidate", s.handleV2SnapshotConsolidate)
	mux.HandleFunc("/v2/memory/search", s.handleV2MemorySearch)
	mux.HandleFunc("/v2/memory/index/rebuild", s.handleV2IndexRebuild)
	mux.HandleFunc("/v2/memory/bookmark/set", s.handleV2BookmarkSet)
	mux.HandleFunc("/v2/memory/abandon", s.handleV2Abandon)
	mux.HandleFunc("/v2/proxy/transform", s.handleV2ProxyTransform)
	mux.HandleFunc("/v2/proxy/event", s.handleProxyEvent)
	mux.HandleFunc("/v2/proxy/compat/report", s.handleCompatReport)
}

func (s *Server) handleV2UseRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
		RepoID   string `json:"repo_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.use_repo", "repo": binding})
}

func (s *Server) handleV2Repos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	items, err := s.store.ListV2Repos(parseIntDefault(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.repos", "repos": items})
}

func (s *Server) handleV2TaskStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
		RepoID   string `json:"repo_id"`
		TaskID   string `json:"task_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.TaskID) != "" {
		if t, err := s.store.GetV2Task(req.TaskID); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.task_start", "task": t, "created": false})
			return
		}
	}
	task, created, err := s.v2.EnsureTask(binding, req.TaskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.task_start", "task": task, "created": created})
}

func (s *Server) handleV2TaskEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		TaskID   string   `json:"task_id"`
		RepoPath string   `json:"repo_path"`
		RepoID   string   `json:"repo_id"`
		Targets  []string `json:"targets"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var task TaskRecord
	var err error
	if strings.TrimSpace(req.TaskID) != "" {
		task, err = s.store.GetV2Task(req.TaskID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	binding, err := s.v2.ResolveRepo(firstNonEmpty(req.RepoPath, task.RepoPath), firstNonEmpty(req.RepoID, task.RepoID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	targets := req.Targets
	if len(targets) == 0 {
		targets = task.TouchedTargets
	}
	if len(targets) == 0 {
		targets = []string{"topic/tooling", "topic/framework", "topic/coding-guidelines"}
	}
	res, err := s.v2.Consolidate(binding, ConsolidateParams{RepoID: binding.RepoID, RepoPath: binding.RepoPath, TaskID: task.TaskID, Targets: targets})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.task_end", "result": res})
}

func (s *Server) handleV2EpisodeCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath      string   `json:"repo_path"`
		RepoID        string   `json:"repo_id"`
		TaskID        string   `json:"task_id"`
		Targets       []string `json:"targets"`
		Source        string   `json:"source"`
		Kind          string   `json:"kind"`
		Confidence    float64  `json:"confidence"`
		ToolDigests   []string `json:"tool_digests"`
		Supersedes    []string `json:"supersedes"`
		StepSummary   string   `json:"step_summary"`
		Facts         []string `json:"facts"`
		Decisions     []string `json:"decisions"`
		Interfaces    []string `json:"interfaces"`
		OpenQuestions []string `json:"open_questions"`
		Evidence      []string `json:"evidence"`
		Notes         []string `json:"notes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	task, _, err := s.v2.EnsureTask(binding, req.TaskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	doc := EpisodeDocument{
		Frontmatter: EpisodeFrontmatter{
			EpisodeID:   ulid.Make().String(),
			CreatedAt:   nowRFC3339Nano(),
			TaskID:      task.TaskID,
			Targets:     dedupeStrings(req.Targets),
			Source:      EpisodeSource(firstNonEmpty(req.Source, string(EpisodeSourceManual))),
			Kind:        EpisodeKind(firstNonEmpty(req.Kind, string(EpisodeKindManualNote))),
			Confidence:  req.Confidence,
			ToolDigests: dedupeStrings(req.ToolDigests),
			Supersedes:  dedupeStrings(req.Supersedes),
		},
		Sections: EpisodeSections{
			StepSummary:   req.StepSummary,
			Facts:         req.Facts,
			Decisions:     req.Decisions,
			Interfaces:    req.Interfaces,
			OpenQuestions: req.OpenQuestions,
			Evidence:      req.Evidence,
			Notes:         req.Notes,
		},
	}
	doc, jjChangeID, jjCommitID, err := s.v2.CreateEpisode(binding, task, doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.episode_create", "episode": doc, "jj_change_id": jjChangeID, "jj_commit_id": jjCommitID})
}

func (s *Server) handleV2EpisodeAbandon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	task, err := s.store.GetV2Task(req.TaskID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(task.RepoPath, task.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_ = s.v2.jj.Abandon(binding, "@")
	_ = s.store.MarkV2TaskEnded(task.TaskID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.episode_abandon", "task_id": task.TaskID})
}

func (s *Server) handleV2SnapshotList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	items, err := s.store.ListV2Snapshots(r.URL.Query().Get("repo_id"), r.URL.Query().Get("target"), r.URL.Query().Get("logical_id"), r.URL.Query().Get("latest_only") != "false", parseIntDefault(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.snapshot_list", "snapshots": items})
}

func (s *Server) handleV2SnapshotRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	binding := RepoBinding{RepoID: r.URL.Query().Get("repo_id")}
	rev := parseIntDefault(r.URL.Query().Get("revision"), 0)
	item, err := s.v2.ReadSnapshotByIDOrLogical(binding, r.URL.Query().Get("snapshot_id"), r.URL.Query().Get("logical_id"), rev)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.snapshot_read", "snapshot": item})
}

func (s *Server) handleV2SnapshotResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath    string   `json:"repo_path"`
		RepoID      string   `json:"repo_id"`
		ContextRefs []string `json:"context_refs"`
		Targets     []string `json:"targets"`
		Query       string   `json:"query"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := s.v2.ResolveSnapshots(binding, dedupeStrings(req.ContextRefs), dedupeStrings(req.Targets), req.Query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.snapshot_resolve", "snapshots": items})
}

func (s *Server) handleV2SnapshotConsolidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string   `json:"repo_path"`
		RepoID   string   `json:"repo_id"`
		Targets  []string `json:"targets"`
		TaskID   string   `json:"task_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.v2.Consolidate(binding, ConsolidateParams{RepoID: binding.RepoID, RepoPath: binding.RepoPath, Targets: dedupeStrings(req.Targets), TaskID: req.TaskID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.snapshot_consolidate", "result": res})
}

func (s *Server) handleV2MemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
		RepoID   string `json:"repo_id"`
		Query    string `json:"query"`
		Limit    int    `json:"limit"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := s.store.SearchV2Snapshots(binding.RepoID, req.Query, req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.search", "snapshots": items})
}

func (s *Server) handleV2IndexRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
		RepoID   string `json:"repo_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, err := s.v2.RebuildIndex(binding)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.index_rebuild", "repo": binding, "indexed": count})
}

func (s *Server) handleV2BookmarkSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
		RepoID   string `json:"repo_id"`
		Name     string `json:"name"`
		Revision string `json:"revision"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.v2.jj.BookmarkSet(binding, req.Name, req.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.bookmark_set", "name": req.Name, "revision": firstNonEmpty(req.Revision, "@")})
}

func (s *Server) handleV2Abandon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		RepoPath string `json:"repo_path"`
		RepoID   string `json:"repo_id"`
		Revision string `json:"revision"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	binding, err := s.v2.ResolveRepo(req.RepoPath, req.RepoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.v2.jj.Abandon(binding, req.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "action": "memory.abandon", "revision": firstNonEmpty(req.Revision, "@")})
}

func (s *Server) handleV2ProxyTransform(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req V2ProxyTransformRequest
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
		writeJSON(w, http.StatusOK, V2ProxyTransformResponse{Status: "success", Injectable: false, Mutated: false, Provider: provider, Reason: "invalid_base64"})
		return
	}
	if isCompressed(req.Headers) {
		_ = s.store.LogProxyEvent(ProxyEvent{Provider: provider, Host: req.Host, Route: req.Path, Injectable: false, Injected: false, Reason: "payload_encrypted_or_compressed"})
		s.emitCompatEpisodeForNonInjectable(req, provider, "payload_encrypted_or_compressed")
		writeJSON(w, http.StatusOK, V2ProxyTransformResponse{Status: "success", Injectable: false, Mutated: false, Provider: provider, BodyBase64: req.BodyBase64, Reason: "payload_encrypted_or_compressed"})
		return
	}
	queryText := extractQueryTextFromJSON(bodyBytes)
	repoPath := firstNonEmpty(req.RepoPath, extractRepoPath(bodyBytes))
	binding, bindErr := s.v2.ResolveRepo(repoPath, req.RepoID)
	if bindErr != nil {
		// fallback to legacy v1 behavior if no repo can be resolved
		searchResp, _ := s.retriever.Search(r.Context(), SearchParams{Query: queryText, Limit: DefaultProxyHintsLimit})
		hint := BuildAgentHint(searchResp.Results)
		outBody, tr := memoryproxy.Transform(provider, req.Path, bodyBytes, hint)
		outB64 := req.BodyBase64
		if tr.Mutated {
			outB64 = base64.StdEncoding.EncodeToString(outBody)
		}
		writeJSON(w, http.StatusOK, V2ProxyTransformResponse{Status: "success", Injectable: tr.Injectable, Mutated: tr.Mutated, Provider: provider, BodyBase64: outB64, Reason: tr.Reason, FallbackUsed: true})
		return
	}
	task, _, taskErr := s.v2.EnsureTask(binding, req.TaskID)
	if taskErr != nil {
		writeError(w, http.StatusInternalServerError, taskErr)
		return
	}
	ctxRefs := dedupeStrings(req.ContextRefs)
	derivedTargets := inferTargetsFromPrompt(queryText, binding.RepoPath)
	resolved, err := s.v2.ResolveSnapshots(binding, ctxRefs, derivedTargets, queryText)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(resolved) == 0 {
		// fallback to legacy hint retrieval during migration
		searchResp, _ := s.retriever.Search(r.Context(), SearchParams{Query: queryText, RepoPath: binding.RepoPath, Limit: DefaultProxyHintsLimit})
		hint := BuildAgentHint(searchResp.Results)
		outBody, tr := memoryproxy.Transform(provider, req.Path, bodyBytes, hint)
		outB64 := req.BodyBase64
		if tr.Mutated {
			outB64 = base64.StdEncoding.EncodeToString(outBody)
		}
		_ = s.store.LogProxyEvent(ProxyEvent{Provider: provider, Host: req.Host, Route: req.Path, Injectable: tr.Injectable, Injected: tr.Mutated, Reason: firstNonEmpty(tr.Reason, "legacy_fallback"), FallbackUsed: true})
		writeJSON(w, http.StatusOK, V2ProxyTransformResponse{Status: "success", Injectable: tr.Injectable, Mutated: tr.Mutated, Provider: provider, RepoID: binding.RepoID, TaskID: task.TaskID, BodyBase64: outB64, Reason: tr.Reason, FallbackUsed: true})
		return
	}
	selected, preamble, resolvedRefs := applySnapshotBudget(resolved, s.v2.budget)
	outBody, tr := memoryproxy.Transform(provider, req.Path, bodyBytes, preamble)
	outB64 := req.BodyBase64
	if tr.Mutated {
		outB64 = base64.StdEncoding.EncodeToString(outBody)
	}
	_ = s.store.LogProxyEvent(ProxyEvent{Provider: provider, Host: req.Host, Route: req.Path, Injectable: tr.Injectable, Injected: tr.Mutated, Reason: tr.Reason})
	if len(selected) > 0 {
		_ = s.store.TouchV2TaskTargets(task.TaskID, uniqueTargetsFromResolved(selected))
	}
	writeJSON(w, http.StatusOK, V2ProxyTransformResponse{Status: "success", Injectable: tr.Injectable, Mutated: tr.Mutated, Provider: provider, RepoID: binding.RepoID, TaskID: task.TaskID, BodyBase64: outB64, Reason: tr.Reason, ResolvedRefs: resolvedRefs, Snapshots: selected, Preamble: hintIfMutated(tr.Mutated, preamble)})
}

func (s *Server) emitCompatEpisodeForNonInjectable(req V2ProxyTransformRequest, provider, reason string) {
	repoPath := strings.TrimSpace(req.RepoPath)
	if repoPath == "" {
		return
	}
	binding, err := s.v2.ResolveRepo(repoPath, req.RepoID)
	if err != nil {
		return
	}
	task, _, err := s.v2.EnsureTask(binding, req.TaskID)
	if err != nil {
		return
	}
	target := "compat/provider:" + firstNonEmpty(provider, req.Host)
	_, _, _, _ = s.v2.CreateEpisode(binding, task, EpisodeDocument{
		Frontmatter: EpisodeFrontmatter{Targets: []string{target}, Source: EpisodeSourceCompat, Kind: EpisodeKindCompatFailure, Confidence: 1.0},
		Sections:    EpisodeSections{StepSummary: "Proxy could not inject memory context", Facts: []string{fmt.Sprintf("Reason: %s", reason), fmt.Sprintf("Host: %s", req.Host), fmt.Sprintf("Route: %s", req.Path)}}})
}

func applySnapshotBudget(in []ResolvedSnapshot, budget TokenBudgetConfig) ([]ResolvedSnapshot, string, []string) {
	if len(in) == 0 {
		return nil, "", nil
	}
	items := append([]ResolvedSnapshot{}, in...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Target != items[j].Target {
			return strings.HasPrefix(items[i].Target, "repo/") && !strings.HasPrefix(items[j].Target, "repo/")
		}
		if items[i].Revision != items[j].Revision {
			return items[i].Revision > items[j].Revision
		}
		return items[i].GeneratedAt > items[j].GeneratedAt
	})
	maxTokens := budget.MaxMemoryBudgetTokens
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	selected := []ResolvedSnapshot{}
	refs := []string{}
	used := 0
	for _, s := range items {
		excerpt := renderSnapshotExcerpt(s)
		toks := approxTokens(excerpt)
		if used > 0 && used+toks > maxTokens {
			continue
		}
		used += toks
		s.Score = float64(used)
		selected = append(selected, s)
		refs = append(refs, s.LogicalID)
	}
	return selected, buildProxyPreamble(selected), dedupeStrings(refs)
}

func buildProxyPreamble(snaps []ResolvedSnapshot) string {
	if len(snaps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Memory Context (Resolved by Proxy)\n")
	b.WriteString("Snapshot refs:\n")
	for _, s := range snaps {
		b.WriteString("- ")
		b.WriteString(s.LogicalID)
		b.WriteString(fmt.Sprintf("@rev%d (%s)\n", s.Revision, s.Target))
	}
	b.WriteString("\n")
	for _, s := range snaps {
		b.WriteString("### ")
		b.WriteString(s.LogicalID)
		b.WriteString("\n")
		appendSectionList(&b, "Decisions", s.Sections.Decisions)
		appendSectionList(&b, "Interfaces", s.Sections.Interfaces)
		appendSectionList(&b, "Facts", s.Sections.Facts)
		appendSectionList(&b, "Open Questions", s.Sections.OpenQuestions)
	}
	b.WriteString("Use this memory context only if it does not conflict with explicit instructions in the current request and repo files.\n")
	return b.String()
}

func renderSnapshotExcerpt(s ResolvedSnapshot) string {
	return strings.Join([]string{
		s.LogicalID,
		strings.Join(s.Sections.Decisions, "\n"),
		strings.Join(s.Sections.Interfaces, "\n"),
		strings.Join(s.Sections.Facts, "\n"),
		strings.Join(s.Sections.OpenQuestions, "\n"),
	}, "\n")
}

func appendSectionList(b *strings.Builder, name string, lines []string) {
	if len(lines) == 0 {
		return
	}
	b.WriteString(name)
	b.WriteString(":\n")
	for _, l := range lines {
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(l))
		b.WriteString("\n")
	}
}

func uniqueTargetsFromResolved(in []ResolvedSnapshot) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.Target)
	}
	return dedupeStrings(out)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
