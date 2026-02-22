package memoryd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

const memoryTimestampFormat = "2006-01-02T15:04:05.000000000Z"

type Store struct {
	db         *sql.DB
	ftsEnabled bool
}

type memoryRow struct {
	Memory
	tagsJSON string
}

func NewStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initSchema() error {
	stmts := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			repo_path TEXT,
			category TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			language TEXT,
			tags_json TEXT NOT NULL,
			source_type TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS embeddings (
			memory_id TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			dims INTEGER NOT NULL,
			vector_json TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS repo_sources (
			repo_path TEXT NOT NULL,
			file_path TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			last_synced_at TEXT NOT NULL,
			PRIMARY KEY(repo_path, file_path)
		);`,
		`CREATE TABLE IF NOT EXISTS proxy_events (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			provider TEXT,
			host TEXT,
			route TEXT,
			injectable INTEGER NOT NULL,
			injected INTEGER NOT NULL,
			fallback_used INTEGER NOT NULL,
			reason TEXT,
			error_redacted TEXT
		);`,
		"CREATE INDEX IF NOT EXISTS idx_memories_scope_active ON memories(scope, active, category, updated_at);",
		"CREATE INDEX IF NOT EXISTS idx_memories_repo_active ON memories(repo_path, active, category, updated_at);",
		"CREATE INDEX IF NOT EXISTS idx_proxy_events_host ON proxy_events(host, route, timestamp);",
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("schema init failed: %w", err)
		}
	}

	// Best-effort FTS5 setup. If unsupported, search falls back to LIKE.
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(memory_id UNINDEXED, title, content, tags);`); err == nil {
		s.ftsEnabled = true
	} else {
		s.ftsEnabled = false
	}

	if err := s.initSchemaV2(); err != nil {
		return err
	}

	return nil
}

func (s *Store) UpsertMemory(params UpsertMemoryParams) (*Memory, error) {
	if err := validateMemoryParams(params); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(params.ID)
	if id == "" {
		id = ulid.Make().String()
	}
	now := nowMemoryTimestamp()
	tags := normalizeTags(params.Tags)
	tagsJSONBytes, _ := json.Marshal(tags)
	active := 0
	if params.Active || params.ID == "" {
		active = 1
	}

	_, err := s.db.Exec(`INSERT INTO memories (id, scope, repo_path, category, title, content, language, tags_json, source_type, active, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			repo_path=excluded.repo_path,
			category=excluded.category,
			title=excluded.title,
			content=excluded.content,
			language=excluded.language,
			tags_json=excluded.tags_json,
			source_type=excluded.source_type,
			active=excluded.active,
			updated_at=excluded.updated_at`,
		id, string(params.Scope), strings.TrimSpace(params.RepoPath), string(params.Category), strings.TrimSpace(params.Title), strings.TrimSpace(params.Content), strings.TrimSpace(params.Language), string(tagsJSONBytes), string(params.SourceType), active, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert memory: %w", err)
	}

	if s.ftsEnabled {
		_, _ = s.db.Exec(`DELETE FROM memory_fts WHERE memory_id = ?`, id)
		if active == 1 {
			_, _ = s.db.Exec(`INSERT INTO memory_fts(memory_id, title, content, tags) VALUES (?, ?, ?, ?)`, id, strings.TrimSpace(params.Title), strings.TrimSpace(params.Content), strings.Join(tags, " "))
		}
	}

	return s.GetMemory(id)
}

func (s *Store) GetMemory(id string) (*Memory, error) {
	row, err := s.getMemoryRow(id)
	if err != nil {
		return nil, err
	}
	mem := row.Memory
	return &mem, nil
}

func (s *Store) getMemoryRow(id string) (*memoryRow, error) {
	var r memoryRow
	var repoPath, language sql.NullString
	var activeInt int
	err := s.db.QueryRow(`SELECT id, scope, repo_path, category, title, content, language, tags_json, source_type, active, created_at, updated_at FROM memories WHERE id = ?`, id).Scan(
		&r.ID, &r.Scope, &repoPath, &r.Category, &r.Title, &r.Content, &language, &r.tagsJSON, &r.SourceType, &activeInt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("memory not found")
		}
		return nil, err
	}
	if repoPath.Valid {
		r.RepoPath = repoPath.String
	}
	if language.Valid {
		r.Language = language.String
	}
	r.Active = activeInt == 1
	_ = json.Unmarshal([]byte(r.tagsJSON), &r.Tags)
	return &r, nil
}

func (s *Store) ListMemories(scope string, repoPath string, limit int) ([]Memory, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, scope, repo_path, category, title, content, language, tags_json, source_type, active, created_at, updated_at FROM memories WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(scope) != "" {
		query += ` AND scope = ?`
		args = append(args, scope)
	}
	if strings.TrimSpace(repoPath) != "" {
		query += ` AND repo_path = ?`
		args = append(args, filepath.Clean(repoPath))
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Memory
	for rows.Next() {
		m, err := scanMemoryRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMemoryRow(scanner interface{ Scan(dest ...any) error }) (Memory, error) {
	var m Memory
	var tagsJSON string
	var repoPath, language sql.NullString
	var activeInt int
	if err := scanner.Scan(&m.ID, &m.Scope, &repoPath, &m.Category, &m.Title, &m.Content, &language, &tagsJSON, &m.SourceType, &activeInt, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return Memory{}, err
	}
	if repoPath.Valid {
		m.RepoPath = repoPath.String
	}
	if language.Valid {
		m.Language = language.String
	}
	m.Active = activeInt == 1
	_ = json.Unmarshal([]byte(tagsJSON), &m.Tags)
	return m, nil
}

func (s *Store) DeleteMemory(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id is required")
	}
	_, err := s.db.Exec(`DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}
	if s.ftsEnabled {
		_, _ = s.db.Exec(`DELETE FROM memory_fts WHERE memory_id = ?`, id)
	}
	return nil
}

func (s *Store) ReplaceRepoSyncMemories(repoPath string, memories []UpsertMemoryParams) (int, error) {
	repoPath = filepath.Clean(strings.TrimSpace(repoPath))
	if repoPath == "" {
		return 0, fmt.Errorf("repo path is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id FROM memories WHERE scope = ? AND repo_path = ? AND source_type = ?`, string(ScopeRepo), repoPath, string(SourceRepoSync))
	if err != nil {
		return 0, err
	}
	var existing []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		existing = append(existing, id)
	}
	rows.Close()

	for _, id := range existing {
		if _, err := tx.Exec(`DELETE FROM memories WHERE id = ?`, id); err != nil {
			return 0, err
		}
	}
	if s.ftsEnabled {
		for _, id := range existing {
			if _, err := tx.Exec(`DELETE FROM memory_fts WHERE memory_id = ?`, id); err != nil {
				return 0, err
			}
		}
	}

	now := nowMemoryTimestamp()
	for _, p := range memories {
		if err := validateMemoryParams(p); err != nil {
			return 0, err
		}
		id := ulid.Make().String()
		tags := normalizeTags(p.Tags)
		tagsJSONBytes, _ := json.Marshal(tags)
		if _, err := tx.Exec(`INSERT INTO memories (id, scope, repo_path, category, title, content, language, tags_json, source_type, active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, 1, ?, ?)`,
			id, string(ScopeRepo), repoPath, string(p.Category), p.Title, p.Content, p.Language, string(tagsJSONBytes), string(SourceRepoSync), now, now); err != nil {
			return 0, err
		}
		if s.ftsEnabled {
			if _, err := tx.Exec(`INSERT INTO memory_fts(memory_id, title, content, tags) VALUES (?, ?, ?, ?)`, id, p.Title, p.Content, strings.Join(tags, " ")); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(memories), nil
}

func (s *Store) SaveEmbedding(memoryID, model string, vector []float64) error {
	if strings.TrimSpace(memoryID) == "" || strings.TrimSpace(model) == "" || len(vector) == 0 {
		return nil
	}
	payload, err := json.Marshal(vector)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO embeddings (memory_id, model, dims, vector_json, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(memory_id) DO UPDATE SET model=excluded.model, dims=excluded.dims, vector_json=excluded.vector_json, updated_at=excluded.updated_at`,
		memoryID, model, len(vector), string(payload), nowMemoryTimestamp())
	return err
}

type EmbeddedMemory struct {
	Memory
	Vector []float64
}

func (s *Store) LoadSearchCandidates(repoPath string, categories []MemoryCategory) ([]EmbeddedMemory, error) {
	query := `SELECT m.id, m.scope, m.repo_path, m.category, m.title, m.content, m.language, m.tags_json, m.source_type, m.active, m.created_at, m.updated_at, e.vector_json
		FROM memories m
		LEFT JOIN embeddings e ON e.memory_id = m.id
		WHERE m.active = 1`
	args := []any{}
	if strings.TrimSpace(repoPath) != "" {
		clean := filepath.Clean(repoPath)
		query += ` AND (m.scope = ? OR (m.scope = ? AND m.repo_path = ?))`
		args = append(args, string(ScopeGlobal), string(ScopeRepo), clean)
	} else {
		query += ` AND m.scope = ?`
		args = append(args, string(ScopeGlobal))
	}
	if len(categories) > 0 {
		placeholders := make([]string, 0, len(categories))
		for _, c := range categories {
			placeholders = append(placeholders, "?")
			args = append(args, string(c))
		}
		query += ` AND m.category IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += ` ORDER BY m.updated_at DESC LIMIT 500`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmbeddedMemory
	for rows.Next() {
		var tagsJSON string
		var repo, lang sql.NullString
		var activeInt int
		var vecJSON sql.NullString
		var m EmbeddedMemory
		if err := rows.Scan(&m.ID, &m.Scope, &repo, &m.Category, &m.Title, &m.Content, &lang, &tagsJSON, &m.SourceType, &activeInt, &m.CreatedAt, &m.UpdatedAt, &vecJSON); err != nil {
			return nil, err
		}
		if repo.Valid {
			m.RepoPath = repo.String
		}
		if lang.Valid {
			m.Language = lang.String
		}
		m.Active = activeInt == 1
		_ = json.Unmarshal([]byte(tagsJSON), &m.Tags)
		if vecJSON.Valid {
			_ = json.Unmarshal([]byte(vecJSON.String), &m.Vector)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SearchFTS(params SearchParams) ([]SearchResult, error) {
	if !s.ftsEnabled {
		return s.searchLike(params)
	}
	limit := params.Limit
	if limit <= 0 {
		limit = DefaultTopK
	}
	queryText := buildFTSQuery(params.Query)
	if queryText == "" {
		return nil, nil
	}
	query := `SELECT m.id, m.scope, m.repo_path, m.category, m.title, m.content, m.language, m.tags_json, m.source_type, m.active, m.created_at, m.updated_at, bm25(memory_fts) AS rank
		FROM memory_fts JOIN memories m ON m.id = memory_fts.memory_id
		WHERE m.active = 1 AND memory_fts MATCH ?`
	args := []any{queryText}
	if strings.TrimSpace(params.RepoPath) != "" {
		clean := filepath.Clean(params.RepoPath)
		query += ` AND (m.scope = ? OR (m.scope = ? AND m.repo_path = ?))`
		args = append(args, string(ScopeGlobal), string(ScopeRepo), clean)
	} else {
		query += ` AND m.scope = ?`
		args = append(args, string(ScopeGlobal))
	}
	if len(params.Categories) > 0 {
		placeholders := make([]string, 0, len(params.Categories))
		for _, c := range params.Categories {
			placeholders = append(placeholders, "?")
			args = append(args, string(c))
		}
		query += ` AND m.category IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += ` ORDER BY rank ASC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return s.searchLike(params)
	}
	defer rows.Close()
	results := make([]SearchResult, 0, limit)
	for rows.Next() {
		var m Memory
		var tagsJSON string
		var repoPath, language sql.NullString
		var activeInt int
		var rank float64
		if err := rows.Scan(&m.ID, &m.Scope, &repoPath, &m.Category, &m.Title, &m.Content, &language, &tagsJSON, &m.SourceType, &activeInt, &m.CreatedAt, &m.UpdatedAt, &rank); err != nil {
			return nil, err
		}
		if repoPath.Valid {
			m.RepoPath = repoPath.String
		}
		if language.Valid {
			m.Language = language.String
		}
		m.Active = activeInt == 1
		_ = json.Unmarshal([]byte(tagsJSON), &m.Tags)
		// bm25 returns smaller-is-better values. Convert to a bounded score.
		score := 0.7
		if rank >= 0 {
			score = 1.0 / (1.0 + rank)
		}
		results = append(results, SearchResult{Memory: m, Score: clampScore(score), RankSource: "fts"})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range results {
		results[i].Score += repoBoost(results[i].Memory, params.RepoPath)
	}
	sortSearchResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *Store) searchLike(params SearchParams) ([]SearchResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = DefaultTopK
	}
	cand, err := s.LoadSearchCandidates(params.RepoPath, params.Categories)
	if err != nil {
		return nil, err
	}
	terms := tokenize(params.Query)
	if len(terms) == 0 {
		return nil, nil
	}
	results := make([]SearchResult, 0, limit)
	for _, c := range cand {
		text := strings.ToLower(c.Title + "\n" + c.Content + "\n" + strings.Join(c.Tags, " "))
		score := 0.0
		matchedTags := []string{}
		for _, t := range terms {
			if strings.Contains(text, t) {
				score += 0.2
			}
			for _, tag := range c.Tags {
				if strings.EqualFold(tag, t) {
					score += 0.25
					matchedTags = append(matchedTags, tag)
				}
			}
		}
		score += repoBoost(c.Memory, params.RepoPath)
		if score <= 0 {
			continue
		}
		results = append(results, SearchResult{Memory: c.Memory, Score: clampScore(score), RankSource: "like", MatchedTags: dedupeStrings(matchedTags)})
	}
	sortSearchResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func sortSearchResults(results []SearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if math.Abs(results[i].Score-results[j].Score) > 1e-9 {
			return results[i].Score > results[j].Score
		}
		if results[i].Memory.Scope != results[j].Memory.Scope {
			return results[i].Memory.Scope == ScopeRepo
		}
		return results[i].Memory.UpdatedAt > results[j].Memory.UpdatedAt
	})
}

func validateMemoryParams(params UpsertMemoryParams) error {
	if params.Scope != ScopeGlobal && params.Scope != ScopeRepo {
		return fmt.Errorf("invalid scope")
	}
	if params.Scope == ScopeRepo && strings.TrimSpace(params.RepoPath) == "" {
		return fmt.Errorf("repo_path is required for repo scope")
	}
	switch params.Category {
	case CategoryTooling, CategoryFramework, CategoryCodingGuideline:
	default:
		return fmt.Errorf("invalid category")
	}
	if strings.TrimSpace(params.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(params.Content) == "" {
		return fmt.Errorf("content is required")
	}
	switch params.SourceType {
	case SourceManual, SourceRepoSync, SourceSeed:
	default:
		return fmt.Errorf("invalid source_type")
	}
	return nil
}

func normalizeTags(tags []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if t == "" {
			continue
		}
		if _, ok := set[t]; ok {
			continue
		}
		set[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func tokenize(input string) []string {
	input = strings.ToLower(input)
	replacer := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "\n", " ", "\t", " ", "/", " ", "\\", " ")
	parts := strings.Fields(replacer.Replace(input))
	set := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) < 2 {
			continue
		}
		if _, ok := set[p]; ok {
			continue
		}
		set[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func buildFTSQuery(query string) string {
	terms := tokenize(query)
	if len(terms) == 0 {
		return ""
	}
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, ``) + `"`
	}
	return strings.Join(terms, " OR ")
}

func repoBoost(m Memory, repoPath string) float64 {
	if strings.TrimSpace(repoPath) == "" {
		return 0
	}
	clean := filepath.Clean(repoPath)
	if m.Scope == ScopeRepo && filepath.Clean(m.RepoPath) == clean {
		return 0.2
	}
	return 0
}

func clampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 0.99 {
		return 0.99
	}
	return v
}

func dedupeStrings(in []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := set[s]; ok {
			continue
		}
		set[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func nowMemoryTimestamp() string {
	return time.Now().UTC().Format(memoryTimestampFormat)
}

func (s *Store) LogProxyEvent(evt ProxyEvent) error {
	if strings.TrimSpace(evt.Timestamp) == "" {
		evt.Timestamp = nowMemoryTimestamp()
	}
	_, err := s.db.Exec(`INSERT INTO proxy_events (id, timestamp, provider, host, route, injectable, injected, fallback_used, reason, error_redacted)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))`,
		ulid.Make().String(), evt.Timestamp, evt.Provider, evt.Host, evt.Route, boolToInt(evt.Injectable), boolToInt(evt.Injected), boolToInt(evt.FallbackUsed), evt.Reason, evt.ErrorRedacted)
	return err
}

func (s *Store) CompatReport(limit int) ([]ProxyCompatRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT COALESCE(provider,''), COALESCE(host,''), COALESCE(route,''), COUNT(*) as requests,
		SUM(CASE WHEN injected = 1 THEN 1 ELSE 0 END) as injected_count,
		SUM(CASE WHEN injectable = 0 THEN 1 ELSE 0 END) as not_injectable_count,
		COALESCE(MAX(reason), ''), MAX(timestamp)
		FROM proxy_events
		GROUP BY provider, host, route
		ORDER BY MAX(timestamp) DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProxyCompatRow
	for rows.Next() {
		var row ProxyCompatRow
		if err := rows.Scan(&row.Provider, &row.Host, &row.Route, &row.Requests, &row.InjectedCount, &row.NotInjectableCount, &row.LastReason, &row.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
