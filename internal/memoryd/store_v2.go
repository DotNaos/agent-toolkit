package memoryd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) initSchemaV2() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS v2_repos (
			repo_id TEXT PRIMARY KEY,
			repo_path TEXT NOT NULL UNIQUE,
			memory_repo_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS v2_tasks (
			task_id TEXT PRIMARY KEY,
			repo_id TEXT NOT NULL,
			repo_path TEXT NOT NULL,
			memory_repo_path TEXT NOT NULL,
			status TEXT NOT NULL,
			bookmark TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			touched_targets_json TEXT NOT NULL DEFAULT '[]'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_v2_tasks_repo_status ON v2_tasks(repo_id, status, started_at);`,
		`CREATE TABLE IF NOT EXISTS v2_episodes (
			episode_id TEXT PRIMARY KEY,
			repo_id TEXT NOT NULL,
			task_id TEXT,
			created_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			source TEXT NOT NULL,
			confidence REAL NOT NULL DEFAULT 0,
			targets_json TEXT NOT NULL,
			relative_path TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_v2_episodes_repo_created ON v2_episodes(repo_id, created_at);`,
		`CREATE TABLE IF NOT EXISTS v2_snapshots (
			snapshot_id TEXT PRIMARY KEY,
			repo_id TEXT NOT NULL,
			logical_id TEXT NOT NULL,
			target TEXT NOT NULL,
			revision INTEGER NOT NULL,
			generated_at TEXT NOT NULL,
			relative_path TEXT NOT NULL,
			raw_markdown TEXT NOT NULL,
			facts_text TEXT,
			decisions_text TEXT,
			interfaces_text TEXT,
			open_questions_text TEXT,
			conflicts_text TEXT,
			evidence_text TEXT,
			source_episode_ids_json TEXT NOT NULL,
			supersedes_snapshot_id TEXT,
			is_latest INTEGER NOT NULL DEFAULT 1
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_v2_snapshots_repo_target_rev ON v2_snapshots(repo_id, target, revision);`,
		`CREATE INDEX IF NOT EXISTS idx_v2_snapshots_repo_latest ON v2_snapshots(repo_id, is_latest, target, generated_at);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("v2 schema init failed: %w", err)
		}
	}
	return nil
}

func (s *Store) UpsertV2Repo(b RepoBinding) error {
	now := nowMemoryTimestamp()
	_, err := s.db.Exec(`INSERT INTO v2_repos (repo_id, repo_path, memory_repo_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET repo_path=excluded.repo_path, memory_repo_path=excluded.memory_repo_path, updated_at=excluded.updated_at`,
		b.RepoID, b.RepoPath, b.MemoryRepoPath, now, now)
	return err
}

func (s *Store) ListV2Repos(limit int) ([]RepoBinding, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT repo_id, repo_path, memory_repo_path, created_at, updated_at FROM v2_repos ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepoBinding
	for rows.Next() {
		var r RepoBinding
		if err := rows.Scan(&r.RepoID, &r.RepoPath, &r.MemoryRepoPath, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetV2RepoByID(repoID string) (RepoBinding, error) {
	var r RepoBinding
	err := s.db.QueryRow(`SELECT repo_id, repo_path, memory_repo_path, created_at, updated_at FROM v2_repos WHERE repo_id = ?`, repoID).Scan(&r.RepoID, &r.RepoPath, &r.MemoryRepoPath, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return RepoBinding{}, err
	}
	return r, nil
}

func (s *Store) UpsertV2Task(t TaskRecord) error {
	targetsJSON, _ := json.Marshal(dedupeStrings(t.TouchedTargets))
	_, err := s.db.Exec(`INSERT INTO v2_tasks (task_id, repo_id, repo_path, memory_repo_path, status, bookmark, started_at, ended_at, touched_targets_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
		ON CONFLICT(task_id) DO UPDATE SET status=excluded.status, bookmark=excluded.bookmark, ended_at=excluded.ended_at, touched_targets_json=excluded.touched_targets_json`,
		t.TaskID, t.RepoID, t.RepoPath, t.MemoryRepoPath, t.Status, t.Bookmark, t.StartedAt, t.EndedAt, string(targetsJSON))
	return err
}

func (s *Store) GetV2Task(taskID string) (TaskRecord, error) {
	var t TaskRecord
	var targetsJSON string
	var endedAt sql.NullString
	err := s.db.QueryRow(`SELECT task_id, repo_id, repo_path, memory_repo_path, status, bookmark, started_at, ended_at, touched_targets_json FROM v2_tasks WHERE task_id = ?`, taskID).Scan(
		&t.TaskID, &t.RepoID, &t.RepoPath, &t.MemoryRepoPath, &t.Status, &t.Bookmark, &t.StartedAt, &endedAt, &targetsJSON,
	)
	if err != nil {
		return TaskRecord{}, err
	}
	if endedAt.Valid {
		t.EndedAt = endedAt.String
	}
	_ = json.Unmarshal([]byte(targetsJSON), &t.TouchedTargets)
	return t, nil
}

func (s *Store) GetActiveV2TaskByRepo(repoID string) (TaskRecord, error) {
	var t TaskRecord
	var targetsJSON string
	var endedAt sql.NullString
	err := s.db.QueryRow(`SELECT task_id, repo_id, repo_path, memory_repo_path, status, bookmark, started_at, ended_at, touched_targets_json
		FROM v2_tasks WHERE repo_id = ? AND status = 'running' ORDER BY started_at DESC LIMIT 1`, repoID).Scan(
		&t.TaskID, &t.RepoID, &t.RepoPath, &t.MemoryRepoPath, &t.Status, &t.Bookmark, &t.StartedAt, &endedAt, &targetsJSON,
	)
	if err != nil {
		return TaskRecord{}, err
	}
	if endedAt.Valid {
		t.EndedAt = endedAt.String
	}
	_ = json.Unmarshal([]byte(targetsJSON), &t.TouchedTargets)
	return t, nil
}

func (s *Store) TouchV2TaskTargets(taskID string, targets []string) error {
	t, err := s.GetV2Task(taskID)
	if err != nil {
		return err
	}
	t.TouchedTargets = dedupeStrings(append(t.TouchedTargets, targets...))
	return s.UpsertV2Task(t)
}

func (s *Store) MarkV2TaskEnded(taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE v2_tasks SET status='ended', ended_at=? WHERE task_id=?`, nowMemoryTimestamp(), taskID)
	return err
}

func (s *Store) UpsertV2Episode(doc EpisodeDocument, relPath string) error {
	targetsJSON, _ := json.Marshal(dedupeStrings(doc.Frontmatter.Targets))
	_, err := s.db.Exec(`INSERT INTO v2_episodes (episode_id, repo_id, task_id, created_at, kind, source, confidence, targets_json, relative_path)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?)
		ON CONFLICT(episode_id) DO UPDATE SET targets_json=excluded.targets_json, relative_path=excluded.relative_path`,
		doc.Frontmatter.EpisodeID, doc.Frontmatter.RepoID, doc.Frontmatter.TaskID, doc.Frontmatter.CreatedAt, string(doc.Frontmatter.Kind), string(doc.Frontmatter.Source), doc.Frontmatter.Confidence, string(targetsJSON), relPath)
	return err
}

func (s *Store) ClearV2SnapshotsForRepo(repoID string) error {
	_, err := s.db.Exec(`DELETE FROM v2_snapshots WHERE repo_id = ?`, repoID)
	return err
}

func (s *Store) UpsertV2Snapshot(repoID, relPath string, doc SnapshotDocument) error {
	if repoID == "" {
		repoID = doc.Frontmatter.RepoID
	}
	// Ensure only latest revision per target is marked latest.
	_, _ = s.db.Exec(`UPDATE v2_snapshots SET is_latest = 0 WHERE repo_id = ? AND target = ?`, repoID, doc.Frontmatter.Target)
	raw := doc.RawMarkdown
	if raw == "" {
		raw = renderSnapshotMarkdown(doc)
	}
	sourceIDsJSON, _ := json.Marshal(dedupeStrings(doc.Frontmatter.SourceEpisodeIDs))
	_, err := s.db.Exec(`INSERT INTO v2_snapshots (snapshot_id, repo_id, logical_id, target, revision, generated_at, relative_path, raw_markdown, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, source_episode_ids_json, supersedes_snapshot_id, is_latest)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), 1)
		ON CONFLICT(snapshot_id) DO UPDATE SET logical_id=excluded.logical_id, target=excluded.target, revision=excluded.revision, generated_at=excluded.generated_at, relative_path=excluded.relative_path, raw_markdown=excluded.raw_markdown, facts_text=excluded.facts_text, decisions_text=excluded.decisions_text, interfaces_text=excluded.interfaces_text, open_questions_text=excluded.open_questions_text, conflicts_text=excluded.conflicts_text, evidence_text=excluded.evidence_text, source_episode_ids_json=excluded.source_episode_ids_json, supersedes_snapshot_id=excluded.supersedes_snapshot_id, is_latest=excluded.is_latest`,
		doc.Frontmatter.SnapshotID, repoID, doc.Frontmatter.LogicalID, doc.Frontmatter.Target, doc.Frontmatter.Revision, doc.Frontmatter.GeneratedAt, relPath, raw,
		strings.Join(doc.Sections.Facts, "\n"), strings.Join(doc.Sections.Decisions, "\n"), strings.Join(doc.Sections.Interfaces, "\n"), strings.Join(doc.Sections.OpenQuestions, "\n"), strings.Join(doc.Sections.ConflictsUncertainty, "\n"), strings.Join(doc.Sections.EvidenceDigests, "\n"), string(sourceIDsJSON), doc.Frontmatter.SupersedesSnapshotID)
	return err
}

func (s *Store) ResolveLatestSnapshotsByTargets(repoID string, targets []string) ([]ResolvedSnapshot, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	q := `SELECT snapshot_id, logical_id, target, revision, generated_at, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, relative_path
		FROM v2_snapshots WHERE repo_id = ? AND is_latest = 1 AND target IN (` + strings.TrimRight(strings.Repeat("?,", len(targets)), ",") + `)`
	args := make([]any, 0, len(targets)+1)
	args = append(args, repoID)
	for _, t := range targets {
		args = append(args, t)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResolvedSnapshots(rows)
}

func (s *Store) ResolveLatestSnapshotsByLogicalIDs(repoID string, logicalIDs []string) ([]ResolvedSnapshot, error) {
	if len(logicalIDs) == 0 {
		return nil, nil
	}
	q := `SELECT snapshot_id, logical_id, target, revision, generated_at, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, relative_path
		FROM v2_snapshots WHERE repo_id = ? AND is_latest = 1 AND logical_id IN (` + strings.TrimRight(strings.Repeat("?,", len(logicalIDs)), ",") + `)`
	args := make([]any, 0, len(logicalIDs)+1)
	args = append(args, repoID)
	for _, t := range logicalIDs {
		args = append(args, t)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResolvedSnapshots(rows)
}

func scanResolvedSnapshots(rows *sql.Rows) ([]ResolvedSnapshot, error) {
	var out []ResolvedSnapshot
	for rows.Next() {
		var r ResolvedSnapshot
		var facts, decisions, ifaces, openQ, conflicts, evidence, relPath string
		if err := rows.Scan(&r.SnapshotID, &r.LogicalID, &r.Target, &r.Revision, &r.GeneratedAt, &facts, &decisions, &ifaces, &openQ, &conflicts, &evidence, &relPath); err != nil {
			return nil, err
		}
		r.Sections = SnapshotSections{
			Facts:                splitNonEmptyLines(facts),
			Decisions:            splitNonEmptyLines(decisions),
			Interfaces:           splitNonEmptyLines(ifaces),
			OpenQuestions:        splitNonEmptyLines(openQ),
			ConflictsUncertainty: splitNonEmptyLines(conflicts),
			EvidenceDigests:      splitNonEmptyLines(evidence),
		}
		r.SourcePath = relPath
		out = append(out, r)
	}
	return out, rows.Err()
}

func splitNonEmptyLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Store) GetV2SnapshotByID(snapshotID string) (*ResolvedSnapshot, error) {
	rows, err := s.db.Query(`SELECT snapshot_id, logical_id, target, revision, generated_at, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, relative_path FROM v2_snapshots WHERE snapshot_id = ? LIMIT 1`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanResolvedSnapshots(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("snapshot not found")
	}
	return &items[0], nil
}

func (s *Store) GetV2SnapshotByLogicalRevision(repoID, logicalID string, revision int) (*ResolvedSnapshot, error) {
	rows, err := s.db.Query(`SELECT snapshot_id, logical_id, target, revision, generated_at, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, relative_path FROM v2_snapshots WHERE repo_id = ? AND logical_id = ? AND revision = ? LIMIT 1`, repoID, logicalID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanResolvedSnapshots(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("snapshot not found")
	}
	return &items[0], nil
}

func (s *Store) ListV2Snapshots(repoID, target, logicalID string, latestOnly bool, limit int) ([]ResolvedSnapshot, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT snapshot_id, logical_id, target, revision, generated_at, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, relative_path FROM v2_snapshots WHERE 1=1`
	args := []any{}
	if repoID != "" {
		q += ` AND repo_id = ?`
		args = append(args, repoID)
	}
	if target != "" {
		q += ` AND target = ?`
		args = append(args, target)
	}
	if logicalID != "" {
		q += ` AND logical_id = ?`
		args = append(args, logicalID)
	}
	if latestOnly {
		q += ` AND is_latest = 1`
	}
	q += ` ORDER BY generated_at DESC, revision DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResolvedSnapshots(rows)
}

func (s *Store) SearchV2Snapshots(repoID, query string, limit int) ([]ResolvedSnapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	like := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(`SELECT snapshot_id, logical_id, target, revision, generated_at, facts_text, decisions_text, interfaces_text, open_questions_text, conflicts_text, evidence_text, relative_path
		FROM v2_snapshots
		WHERE repo_id = ? AND is_latest = 1 AND (
			LOWER(logical_id) LIKE ? OR LOWER(target) LIKE ? OR LOWER(raw_markdown) LIKE ?
		)
		ORDER BY generated_at DESC LIMIT ?`, repoID, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResolvedSnapshots(rows)
}
