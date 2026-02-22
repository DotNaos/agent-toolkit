package memoryd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oklog/ulid/v2"
)

type V2Service struct {
	store           *Store
	retriever       *Retriever
	jj              *JJManager
	budget          TokenBudgetConfig
	memoryReposRoot string
}

func NewV2Service(store *Store, retriever *Retriever) *V2Service {
	return NewV2ServiceWithRoot(store, retriever, "")
}

func NewV2ServiceWithRoot(store *Store, retriever *Retriever, root string) *V2Service {
	jj := NewJJManager(root)
	return &V2Service{store: store, retriever: retriever, jj: jj, budget: DefaultTokenBudget(), memoryReposRoot: jj.MemoryReposRoot()}
}

func (v *V2Service) ResolveRepo(repoPath, repoID string) (RepoBinding, error) {
	if strings.TrimSpace(repoPath) != "" {
		b, err := v.jj.ResolveBinding(repoPath)
		if err != nil {
			return RepoBinding{}, err
		}
		if err := v.jj.EnsureRepo(b); err != nil {
			return RepoBinding{}, err
		}
		_ = v.store.UpsertV2Repo(b)
		return b, nil
	}
	if strings.TrimSpace(repoID) != "" {
		b, err := v.store.GetV2RepoByID(repoID)
		if err == nil {
			_ = v.jj.EnsureRepo(b)
			return b, nil
		}
	}
	return RepoBinding{}, fmt.Errorf("repo_path or repo_id is required")
}

func (v *V2Service) EnsureTask(binding RepoBinding, taskID string) (TaskRecord, bool, error) {
	if strings.TrimSpace(taskID) != "" {
		if t, err := v.store.GetV2Task(taskID); err == nil {
			return t, false, nil
		}
	}
	if t, err := v.store.GetActiveV2TaskByRepo(binding.RepoID); err == nil {
		return t, false, nil
	}
	newTaskID := ulid.Make().String()
	t, err := v.jj.TaskStart(binding, newTaskID)
	if err != nil {
		return TaskRecord{}, false, err
	}
	if err := v.store.UpsertV2Task(t); err != nil {
		return TaskRecord{}, false, err
	}
	return t, true, nil
}

func (v *V2Service) CreateEpisode(binding RepoBinding, task TaskRecord, doc EpisodeDocument) (EpisodeDocument, string, string, error) {
	if doc.Frontmatter.EpisodeID == "" {
		doc.Frontmatter.EpisodeID = ulid.Make().String()
	}
	if doc.Frontmatter.CreatedAt == "" {
		doc.Frontmatter.CreatedAt = nowRFC3339Nano()
	}
	doc.Frontmatter.RepoID = binding.RepoID
	doc.Frontmatter.RepoPath = binding.RepoPath
	if task.TaskID != "" && doc.Frontmatter.TaskID == "" {
		doc.Frontmatter.TaskID = task.TaskID
	}
	doc.Frontmatter.Targets = dedupeStrings(doc.Frontmatter.Targets)
	if len(doc.Frontmatter.Targets) == 0 {
		doc.Frontmatter.Targets = []string{"task/" + task.TaskID}
	}
	relPath := episodeRelativePath(doc)
	absPath := filepath.Join(binding.MemoryRepoPath, filepath.FromSlash(relPath))
	if err := writeEpisodeDoc(absPath, doc); err != nil {
		return EpisodeDocument{}, "", "", err
	}
	chg, commit, err := v.jj.CommitFileAtom(binding, relPath, fmt.Sprintf("episode(%s): %s", sanitizePathSegment(doc.Frontmatter.Targets[0]), truncateForSubject(doc.Sections.StepSummary, 60)))
	if err != nil {
		return EpisodeDocument{}, "", "", err
	}
	doc.Path = absPath
	_ = v.store.UpsertV2Episode(doc, relPath)
	if task.TaskID != "" {
		_ = v.store.TouchV2TaskTargets(task.TaskID, doc.Frontmatter.Targets)
	}
	return doc, chg, commit, nil
}

func (v *V2Service) Consolidate(binding RepoBinding, params ConsolidateParams) (ConsolidateResult, error) {
	episodeFiles, err := listEpisodeFiles(binding.MemoryRepoPath)
	if err != nil {
		return ConsolidateResult{}, err
	}
	byTarget := map[string][]EpisodeDocument{}
	for _, file := range episodeFiles {
		ep, err := readEpisodeDoc(file)
		if err != nil {
			continue
		}
		if ep.Frontmatter.RepoID != "" && ep.Frontmatter.RepoID != binding.RepoID {
			continue
		}
		for _, target := range ep.Frontmatter.Targets {
			if len(params.Targets) > 0 && !containsString(params.Targets, target) {
				continue
			}
			byTarget[target] = append(byTarget[target], ep)
		}
	}
	result := ConsolidateResult{RepoID: binding.RepoID, Targets: append([]string{}, params.Targets...)}
	if len(result.Targets) == 0 {
		result.Targets = sortedKeys(byTarget)
	}
	for _, target := range result.Targets {
		eps := byTarget[target]
		if len(eps) == 0 {
			result.SkippedTargets = append(result.SkippedTargets, target)
			continue
		}
		sort.SliceStable(eps, func(i, j int) bool {
			return eps[i].Frontmatter.CreatedAt < eps[j].Frontmatter.CreatedAt
		})
		snap, relPath, err := v.buildSnapshotForTarget(binding, target, eps)
		if err != nil {
			result.SkippedTargets = append(result.SkippedTargets, target)
			continue
		}
		absPath := filepath.Join(binding.MemoryRepoPath, filepath.FromSlash(relPath))
		if err := writeSnapshotDoc(absPath, snap); err != nil {
			result.SkippedTargets = append(result.SkippedTargets, target)
			continue
		}
		_, _, _ = v.jj.CommitFileAtom(binding, relPath, fmt.Sprintf("snapshot(%s): rev-%d", sanitizePathSegment(target), snap.Frontmatter.Revision))
		resolved := ResolvedSnapshot{
			SnapshotID:  snap.Frontmatter.SnapshotID,
			LogicalID:   snap.Frontmatter.LogicalID,
			Target:      target,
			Revision:    snap.Frontmatter.Revision,
			GeneratedAt: snap.Frontmatter.GeneratedAt,
			Sections:    snap.Sections,
			SourcePath:  absPath,
		}
		result.Generated = append(result.Generated, resolved)
		_ = v.store.UpsertV2Snapshot(binding.RepoID, relPath, snap)
	}
	if len(result.Generated) > 0 {
		_ = v.store.MarkV2TaskEnded(params.TaskID)
		_ = v.jj.BookmarkSet(binding, "main", "@-")
	}
	return result, nil
}

func (v *V2Service) buildSnapshotForTarget(binding RepoBinding, target string, eps []EpisodeDocument) (SnapshotDocument, string, error) {
	nextRev, latestRel, err := nextSnapshotRevision(binding.MemoryRepoPath, target)
	if err != nil {
		return SnapshotDocument{}, "", err
	}
	prevSnapshotID := ""
	if latestRel != "" {
		if prev, err := readSnapshotDoc(filepath.Join(binding.MemoryRepoPath, filepath.FromSlash(latestRel))); err == nil {
			prevSnapshotID = prev.Frontmatter.SnapshotID
		}
	}
	facts, factConflicts := mergeStatementsByKey(eps, func(e EpisodeDocument) []string { return e.Sections.Facts })
	decisions, decisionConflicts := mergeStatementsByKey(eps, func(e EpisodeDocument) []string { return e.Sections.Decisions })
	interfaces, _ := mergeStatementsByKey(eps, func(e EpisodeDocument) []string { return e.Sections.Interfaces })
	openQuestions, _ := mergeStatementsByKey(eps, func(e EpisodeDocument) []string { return e.Sections.OpenQuestions })
	evidence, _ := mergeStatementsByKey(eps, func(e EpisodeDocument) []string { return e.Sections.Evidence })
	conflicts := append(factConflicts, decisionConflicts...)
	logicalID := logicalIDFromTarget(target)
	snap := SnapshotDocument{
		Frontmatter: SnapshotFrontmatter{
			SnapshotID:           ulid.Make().String(),
			LogicalID:            logicalID,
			Revision:             nextRev,
			RepoID:               binding.RepoID,
			Target:               target,
			GeneratedAt:          nowRFC3339Nano(),
			SourceEpisodeIDs:     collectEpisodeIDs(eps),
			ConflictPolicy:       "newest-wins+mark",
			SupersedesSnapshotID: prevSnapshotID,
		},
		Sections: SnapshotSections{
			Facts:                facts,
			Decisions:            decisions,
			Interfaces:           interfaces,
			OpenQuestions:        openQuestions,
			ConflictsUncertainty: dedupeStrings(conflicts),
			EvidenceDigests:      evidence,
		},
	}
	relPath := snapshotRelativePath(target, nextRev)
	return snap, relPath, nil
}

func mergeStatementsByKey(eps []EpisodeDocument, selector func(EpisodeDocument) []string) ([]string, []string) {
	type item struct{ text, when, episode string }
	latestByKey := map[string]item{}
	order := []string{}
	conflicts := []string{}
	for _, ep := range eps {
		for _, raw := range selector(ep) {
			text := strings.TrimSpace(raw)
			if text == "" {
				continue
			}
			key := statementKey(text)
			cur := item{text: text, when: ep.Frontmatter.CreatedAt, episode: ep.Frontmatter.EpisodeID}
			if prev, ok := latestByKey[key]; ok {
				if prev.text != text {
					if prev.when <= cur.when {
						conflicts = append(conflicts, fmt.Sprintf("%s: superseded %q with %q (episodes %s -> %s)", key, prev.text, cur.text, prev.episode, cur.episode))
						latestByKey[key] = cur
					} else {
						conflicts = append(conflicts, fmt.Sprintf("%s: kept newer %q, ignored %q (episodes %s vs %s)", key, prev.text, cur.text, prev.episode, cur.episode))
					}
				}
				continue
			}
			latestByKey[key] = cur
			order = append(order, key)
		}
	}
	out := []string{}
	for _, k := range order {
		if it, ok := latestByKey[k]; ok {
			out = append(out, it.text)
		}
	}
	return dedupeStrings(out), dedupeStrings(conflicts)
}

func statementKey(text string) string {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	if idx := strings.Index(lower, ":"); idx > 0 && idx < 40 {
		return strings.TrimSpace(lower[:idx])
	}
	// Normalize common preference statements like "Use bun ..." => "use"
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 1 && (fields[0] == "use" || fields[0] == "prefer") {
		return fields[0] + ":" + fields[1]
	}
	if len(lower) > 80 {
		lower = lower[:80]
	}
	return lower
}

func logicalIDFromTarget(target string) string {
	switch {
	case strings.HasPrefix(target, "topic/"):
		return "mem/" + target
	case strings.HasPrefix(target, "repo/file:"):
		return "mem/file#" + strings.TrimPrefix(target, "repo/file:")
	case strings.HasPrefix(target, "repo/symbol:"):
		return "mem/symbol#" + strings.TrimPrefix(target, "repo/symbol:")
	case strings.HasPrefix(target, "compat/"):
		return "mem/" + target
	case strings.HasPrefix(target, "task/"):
		return "mem/" + target
	default:
		return "mem/target#" + target
	}
}

func collectEpisodeIDs(eps []EpisodeDocument) []string {
	ids := make([]string, 0, len(eps))
	for _, ep := range eps {
		if ep.Frontmatter.EpisodeID != "" {
			ids = append(ids, ep.Frontmatter.EpisodeID)
		}
	}
	return dedupeStrings(ids)
}

func containsString(list []string, needle string) bool {
	for _, s := range list {
		if s == needle {
			return true
		}
	}
	return false
}

func truncateForSubject(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "update"
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func inferTargetsFromPrompt(query string, repoPath string) []string {
	queryLower := strings.ToLower(query)
	targets := []string{}
	if strings.Contains(queryLower, "bun") || strings.Contains(queryLower, "npm") || strings.Contains(queryLower, "pnpm") || strings.Contains(queryLower, "yarn") || strings.Contains(queryLower, "uv") || strings.Contains(queryLower, "pip") {
		targets = append(targets, "topic/tooling")
	}
	if strings.Contains(queryLower, "react") || strings.Contains(queryLower, "next") || strings.Contains(queryLower, "tailwind") || strings.Contains(queryLower, "fastapi") || strings.Contains(queryLower, "django") {
		targets = append(targets, "topic/framework")
	}
	if strings.Contains(queryLower, "guideline") || strings.Contains(queryLower, "style") || strings.Contains(queryLower, "should") || strings.Contains(queryLower, "must") || strings.Contains(queryLower, "prefer") {
		targets = append(targets, "topic/coding-guidelines")
	}
	if repoPath != "" {
		// file path targeting from request body strings
		if p := extractRepoPathFromPrompt(queryLower, repoPath); p != "" {
			targets = append(targets, "repo/file:"+p)
		}
	}
	return dedupeStrings(targets)
}

func extractRepoPathFromPrompt(query, repoRoot string) string {
	_ = query
	// For now we target the repo root when a repo is known; symbol/file extraction can be added incrementally.
	return filepath.Clean(repoRoot)
}

func (v *V2Service) ResolveSnapshots(binding RepoBinding, logicalIDs []string, targets []string, query string) ([]ResolvedSnapshot, error) {
	if len(logicalIDs) > 0 {
		return v.store.ResolveLatestSnapshotsByLogicalIDs(binding.RepoID, logicalIDs)
	}
	if len(targets) == 0 {
		targets = inferTargetsFromPrompt(query, binding.RepoPath)
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return v.store.ResolveLatestSnapshotsByTargets(binding.RepoID, targets)
}

func (v *V2Service) RebuildIndex(binding RepoBinding) (int, error) {
	files, err := listSnapshotFiles(binding.MemoryRepoPath)
	if err != nil {
		return 0, err
	}
	_ = v.store.ClearV2SnapshotsForRepo(binding.RepoID)
	count := 0
	for _, file := range files {
		doc, err := readSnapshotDoc(file)
		if err != nil {
			continue
		}
		if doc.Frontmatter.RepoID == "" {
			doc.Frontmatter.RepoID = binding.RepoID
		}
		rel, _ := filepath.Rel(binding.MemoryRepoPath, file)
		rel = filepath.ToSlash(rel)
		if err := v.store.UpsertV2Snapshot(binding.RepoID, rel, doc); err == nil {
			count++
		}
	}
	return count, nil
}

func (v *V2Service) ReadSnapshotByIDOrLogical(binding RepoBinding, snapshotID, logicalID string, revision int) (*ResolvedSnapshot, error) {
	if strings.TrimSpace(snapshotID) != "" {
		return v.store.GetV2SnapshotByID(snapshotID)
	}
	if strings.TrimSpace(logicalID) == "" {
		return nil, fmt.Errorf("snapshot_id or logical_id is required")
	}
	if revision > 0 {
		return v.store.GetV2SnapshotByLogicalRevision(binding.RepoID, logicalID, revision)
	}
	items, err := v.store.ResolveLatestSnapshotsByLogicalIDs(binding.RepoID, []string{logicalID})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("snapshot not found")
	}
	return &items[0], nil
}

func ensureFileExists(path string) error {
	_, err := os.Stat(path)
	return err
}
