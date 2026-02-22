package memoryd

import "time"

const (
	DefaultListenAddr      = "127.0.0.1:45229"
	DefaultOllamaURL       = "http://127.0.0.1:11434"
	DefaultEmbeddingModel  = "nomic-embed-text"
	DefaultTopK            = 3
	DefaultScoreThreshold  = 0.62
	DefaultProxyHintsLimit = 3
)

type MemoryScope string

type MemoryCategory string

type MemorySourceType string

const (
	ScopeGlobal MemoryScope = "global"
	ScopeRepo   MemoryScope = "repo"

	CategoryTooling         MemoryCategory = "tooling"
	CategoryFramework       MemoryCategory = "framework"
	CategoryCodingGuideline MemoryCategory = "coding-guideline"

	SourceManual   MemorySourceType = "manual"
	SourceRepoSync MemorySourceType = "repo-sync"
	SourceSeed     MemorySourceType = "seed"
)

type Memory struct {
	ID         string           `json:"id"`
	Scope      MemoryScope      `json:"scope"`
	RepoPath   string           `json:"repo_path,omitempty"`
	Category   MemoryCategory   `json:"category"`
	Title      string           `json:"title"`
	Content    string           `json:"content"`
	Language   string           `json:"language,omitempty"`
	Tags       []string         `json:"tags,omitempty"`
	SourceType MemorySourceType `json:"source_type"`
	Active     bool             `json:"active"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
}

type UpsertMemoryParams struct {
	ID         string
	Scope      MemoryScope
	RepoPath   string
	Category   MemoryCategory
	Title      string
	Content    string
	Language   string
	Tags       []string
	SourceType MemorySourceType
	Active     bool
}

type SearchParams struct {
	Query          string
	RepoPath       string
	Categories     []MemoryCategory
	Limit          int
	ScoreThreshold float64
}

type SearchResult struct {
	Memory      Memory   `json:"memory"`
	Score       float64  `json:"score"`
	RankSource  string   `json:"rank_source"`
	MatchedTags []string `json:"matched_tags,omitempty"`
}

type SearchResponse struct {
	Results       []SearchResult `json:"results"`
	FallbackUsed  bool           `json:"fallback_used"`
	EmbeddingUsed bool           `json:"embedding_used"`
	Reason        string         `json:"reason,omitempty"`
}

type ProxyTransformRequest struct {
	Provider   string            `json:"provider"`
	Host       string            `json:"host"`
	Path       string            `json:"path"`
	Method     string            `json:"method,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	BodyBase64 string            `json:"body_b64"`
}

type ProxyTransformResponse struct {
	Status       string `json:"status"`
	Injectable   bool   `json:"injectable"`
	Mutated      bool   `json:"mutated"`
	BodyBase64   string `json:"body_b64,omitempty"`
	Hint         string `json:"hint,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Provider     string `json:"provider,omitempty"`
	FallbackUsed bool   `json:"fallback_used,omitempty"`
}

type ProxyEvent struct {
	Timestamp     string `json:"timestamp,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Host          string `json:"host,omitempty"`
	Route         string `json:"route,omitempty"`
	Injectable    bool   `json:"injectable"`
	Injected      bool   `json:"injected"`
	FallbackUsed  bool   `json:"fallback_used"`
	Reason        string `json:"reason,omitempty"`
	ErrorRedacted string `json:"error_redacted,omitempty"`
}

type ProxyCompatRow struct {
	Provider           string `json:"provider"`
	Host               string `json:"host"`
	Route              string `json:"route"`
	Requests           int64  `json:"requests"`
	InjectedCount      int64  `json:"injected_count"`
	NotInjectableCount int64  `json:"not_injectable_count"`
	LastReason         string `json:"last_reason,omitempty"`
	LastSeenAt         string `json:"last_seen_at,omitempty"`
}

type ServerConfig struct {
	ListenAddr      string
	DBPath          string
	OllamaURL       string
	EmbeddingModel  string
	ScoreThreshold  float64
	MemoryReposRoot string
}

type TimestampProvider func() time.Time
