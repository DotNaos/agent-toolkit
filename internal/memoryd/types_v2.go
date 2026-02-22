package memoryd

import "time"

type RepoBinding struct {
	RepoID         string `json:"repo_id"`
	RepoPath       string `json:"repo_path"`
	MemoryRepoPath string `json:"memory_repo_path"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type TaskRecord struct {
	TaskID         string   `json:"task_id"`
	RepoID         string   `json:"repo_id"`
	RepoPath       string   `json:"repo_path"`
	MemoryRepoPath string   `json:"memory_repo_path"`
	Status         string   `json:"status"`
	Bookmark       string   `json:"bookmark"`
	StartedAt      string   `json:"started_at"`
	EndedAt        string   `json:"ended_at,omitempty"`
	TouchedTargets []string `json:"touched_targets,omitempty"`
}

type EpisodeSource string

type EpisodeKind string

const (
	EpisodeSourceProxy         EpisodeSource = "proxy"
	EpisodeSourceManual        EpisodeSource = "manual"
	EpisodeSourceConsolidation EpisodeSource = "consolidation"
	EpisodeSourceCompat        EpisodeSource = "compat"

	EpisodeKindTaskStep      EpisodeKind = "task-step"
	EpisodeKindProxyEvent    EpisodeKind = "proxy-event"
	EpisodeKindCompatFailure EpisodeKind = "compat-failure"
	EpisodeKindManualNote    EpisodeKind = "manual-note"
	EpisodeKindConsolidation EpisodeKind = "consolidation"
)

type EpisodeFrontmatter struct {
	EpisodeID   string        `json:"episode_id"`
	CreatedAt   string        `json:"created_at"`
	TaskID      string        `json:"task_id,omitempty"`
	RepoID      string        `json:"repo_id"`
	RepoPath    string        `json:"repo_path"`
	Targets     []string      `json:"targets"`
	Source      EpisodeSource `json:"source"`
	Kind        EpisodeKind   `json:"kind"`
	Confidence  float64       `json:"confidence,omitempty"`
	ToolDigests []string      `json:"tool_digests,omitempty"`
	Supersedes  []string      `json:"supersedes,omitempty"`
}

type EpisodeSections struct {
	StepSummary   string   `json:"step_summary,omitempty"`
	Facts         []string `json:"facts,omitempty"`
	Decisions     []string `json:"decisions,omitempty"`
	Interfaces    []string `json:"interfaces,omitempty"`
	OpenQuestions []string `json:"open_questions,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

type EpisodeDocument struct {
	Frontmatter EpisodeFrontmatter `json:"frontmatter"`
	Sections    EpisodeSections    `json:"sections"`
	Path        string             `json:"path,omitempty"`
}

type SnapshotFrontmatter struct {
	SnapshotID           string   `json:"snapshot_id"`
	LogicalID            string   `json:"logical_id"`
	Revision             int      `json:"revision"`
	RepoID               string   `json:"repo_id"`
	Target               string   `json:"target"`
	GeneratedAt          string   `json:"generated_at"`
	SourceEpisodeIDs     []string `json:"source_episode_ids,omitempty"`
	ConflictPolicy       string   `json:"conflict_policy"`
	SupersedesSnapshotID string   `json:"supersedes_snapshot_id,omitempty"`
}

type SnapshotSections struct {
	Facts                []string `json:"facts,omitempty"`
	Decisions            []string `json:"decisions,omitempty"`
	Interfaces           []string `json:"interfaces,omitempty"`
	OpenQuestions        []string `json:"open_questions,omitempty"`
	ConflictsUncertainty []string `json:"conflicts_uncertainty,omitempty"`
	EvidenceDigests      []string `json:"evidence_digests,omitempty"`
}

type SnapshotDocument struct {
	Frontmatter SnapshotFrontmatter `json:"frontmatter"`
	Sections    SnapshotSections    `json:"sections"`
	Path        string              `json:"path,omitempty"`
	RawMarkdown string              `json:"raw_markdown,omitempty"`
}

type ResolvedSnapshot struct {
	SnapshotID  string           `json:"snapshot_id"`
	LogicalID   string           `json:"logical_id"`
	Target      string           `json:"target"`
	Revision    int              `json:"revision"`
	GeneratedAt string           `json:"generated_at"`
	Sections    SnapshotSections `json:"sections"`
	SourcePath  string           `json:"source_path,omitempty"`
	Score       float64          `json:"score,omitempty"`
	Reason      string           `json:"reason,omitempty"`
}

type ConsolidateParams struct {
	RepoID   string
	RepoPath string
	Targets  []string
	TaskID   string
}

type ConsolidateResult struct {
	RepoID         string             `json:"repo_id"`
	Targets        []string           `json:"targets"`
	Generated      []ResolvedSnapshot `json:"generated"`
	SkippedTargets []string           `json:"skipped_targets,omitempty"`
}

type V2ProxyTransformRequest struct {
	Provider    string            `json:"provider,omitempty"`
	Host        string            `json:"host,omitempty"`
	Path        string            `json:"path"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyBase64  string            `json:"body_b64"`
	RepoPath    string            `json:"repo_path,omitempty"`
	RepoID      string            `json:"repo_id,omitempty"`
	TaskID      string            `json:"task_id,omitempty"`
	ContextRefs []string          `json:"context_refs,omitempty"`
	Model       string            `json:"model,omitempty"`
}

type V2ProxyTransformResponse struct {
	Status       string             `json:"status"`
	Injectable   bool               `json:"injectable"`
	Mutated      bool               `json:"mutated"`
	BodyBase64   string             `json:"body_b64,omitempty"`
	Reason       string             `json:"reason,omitempty"`
	Provider     string             `json:"provider,omitempty"`
	RepoID       string             `json:"repo_id,omitempty"`
	TaskID       string             `json:"task_id,omitempty"`
	ResolvedRefs []string           `json:"resolved_refs,omitempty"`
	Snapshots    []ResolvedSnapshot `json:"snapshots,omitempty"`
	Preamble     string             `json:"preamble,omitempty"`
	FallbackUsed bool               `json:"fallback_used,omitempty"`
}

type TokenBudgetConfig struct {
	MaxContextTokens      int
	ReservedUserTokens    int
	MaxMemoryBudgetTokens int
}

func DefaultTokenBudget() TokenBudgetConfig {
	return TokenBudgetConfig{
		MaxContextTokens:      128000,
		ReservedUserTokens:    100000,
		MaxMemoryBudgetTokens: 8000,
	}
}

func approxTokens(s string) int {
	if s == "" {
		return 0
	}
	return len(s)/4 + 1
}

type clockFn func() time.Time
