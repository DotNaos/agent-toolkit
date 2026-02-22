package memoryd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func episodeRelativePath(ep EpisodeDocument) string {
	ts := ep.Frontmatter.CreatedAt
	if ts == "" {
		ts = nowRFC3339Nano()
	}
	date := ts
	if len(date) >= 10 {
		date = date[:10]
	}
	parts := strings.Split(date, "-")
	if len(parts) != 3 {
		parts = []string{"0000", "00", "00"}
	}
	return filepath.ToSlash(filepath.Join("episodes", parts[0], parts[1], parts[2], ep.Frontmatter.EpisodeID+".md"))
}

func targetPathParts(target string) []string {
	if strings.HasPrefix(target, "topic/") || strings.HasPrefix(target, "task/") || strings.HasPrefix(target, "compat/") {
		parts := strings.Split(target, "/")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, sanitizePathSegment(p))
		}
		return out
	}
	return []string{"targets", sanitizePathSegment(target)}
}

func snapshotDirForTarget(target string) string {
	parts := append([]string{"snapshots"}, targetPathParts(target)...)
	return filepath.ToSlash(filepath.Join(parts...))
}

func snapshotRelativePath(target string, rev int) string {
	return filepath.ToSlash(filepath.Join(snapshotDirForTarget(target), fmt.Sprintf("rev-%d.md", rev)))
}

func sanitizePathSegment(s string) string {
	if s == "" {
		return "_"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	s = re.ReplaceAllString(s, "_")
	s = strings.Trim(s, "._-")
	if s == "" {
		return "_"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func nextSnapshotRevision(memoryRepoPath, target string) (int, string, error) {
	dir := filepath.Join(memoryRepoPath, snapshotDirForTarget(target))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, "", err
	}
	maxRev := 0
	latestPath := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rev-") || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		num := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "rev-"), ".md")
		rev, err := strconv.Atoi(num)
		if err != nil {
			continue
		}
		if rev > maxRev {
			maxRev = rev
			latestPath = filepath.ToSlash(filepath.Join(snapshotDirForTarget(target), e.Name()))
		}
	}
	return maxRev + 1, latestPath, nil
}

func writeEpisodeDoc(absPath string, doc EpisodeDocument) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(renderEpisodeMarkdown(doc)), 0o644)
}

func writeSnapshotDoc(absPath string, doc SnapshotDocument) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	md := renderSnapshotMarkdown(doc)
	return os.WriteFile(absPath, []byte(md), 0o644)
}

func renderEpisodeMarkdown(doc EpisodeDocument) string {
	b := &strings.Builder{}
	front, _ := json.MarshalIndent(doc.Frontmatter, "", "  ")
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("\n---\n\n")
	writeSection(b, "Step Summary", []string{doc.Sections.StepSummary}, false)
	writeSection(b, "Facts", doc.Sections.Facts, true)
	writeSection(b, "Decisions", doc.Sections.Decisions, true)
	writeSection(b, "Interfaces", doc.Sections.Interfaces, true)
	writeSection(b, "Open Questions", doc.Sections.OpenQuestions, true)
	writeSection(b, "Evidence", doc.Sections.Evidence, true)
	writeSection(b, "Notes", doc.Sections.Notes, true)
	return b.String()
}

func renderSnapshotMarkdown(doc SnapshotDocument) string {
	b := &strings.Builder{}
	front, _ := json.MarshalIndent(doc.Frontmatter, "", "  ")
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("\n---\n\n")
	writeSection(b, "Facts", doc.Sections.Facts, true)
	writeSection(b, "Decisions", doc.Sections.Decisions, true)
	writeSection(b, "Interfaces", doc.Sections.Interfaces, true)
	writeSection(b, "Open Questions", doc.Sections.OpenQuestions, true)
	writeSection(b, "Conflicts / Uncertainty", doc.Sections.ConflictsUncertainty, true)
	writeSection(b, "Evidence Digests", doc.Sections.EvidenceDigests, true)
	return b.String()
}

func writeSection(b *strings.Builder, title string, lines []string, bullets bool) {
	clean := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	if len(clean) == 0 {
		return
	}
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, line := range clean {
		if bullets {
			b.WriteString("- ")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func parseEpisodeMarkdown(raw string) (EpisodeDocument, error) {
	frontBytes, body, err := splitFrontmatter(raw)
	if err != nil {
		return EpisodeDocument{}, err
	}
	var fm EpisodeFrontmatter
	if err := json.Unmarshal(frontBytes, &fm); err != nil {
		return EpisodeDocument{}, fmt.Errorf("invalid episode frontmatter: %w", err)
	}
	sections := parseSections(body)
	return EpisodeDocument{Frontmatter: fm, Sections: EpisodeSections{
		StepSummary:   firstLine(sections["Step Summary"]),
		Facts:         normalizeList(sections["Facts"]),
		Decisions:     normalizeList(sections["Decisions"]),
		Interfaces:    normalizeList(sections["Interfaces"]),
		OpenQuestions: normalizeList(sections["Open Questions"]),
		Evidence:      normalizeList(sections["Evidence"]),
		Notes:         normalizeList(sections["Notes"]),
	}}, nil
}

func parseSnapshotMarkdown(raw string) (SnapshotDocument, error) {
	frontBytes, body, err := splitFrontmatter(raw)
	if err != nil {
		return SnapshotDocument{}, err
	}
	var fm SnapshotFrontmatter
	if err := json.Unmarshal(frontBytes, &fm); err != nil {
		return SnapshotDocument{}, fmt.Errorf("invalid snapshot frontmatter: %w", err)
	}
	sections := parseSections(body)
	return SnapshotDocument{Frontmatter: fm, Sections: SnapshotSections{
		Facts:                normalizeList(sections["Facts"]),
		Decisions:            normalizeList(sections["Decisions"]),
		Interfaces:           normalizeList(sections["Interfaces"]),
		OpenQuestions:        normalizeList(sections["Open Questions"]),
		ConflictsUncertainty: normalizeList(sections["Conflicts / Uncertainty"]),
		EvidenceDigests:      normalizeList(sections["Evidence Digests"]),
	}, RawMarkdown: raw}, nil
}

func splitFrontmatter(raw string) ([]byte, string, error) {
	if !strings.HasPrefix(raw, "---\n") {
		return nil, "", fmt.Errorf("missing frontmatter start")
	}
	rest := strings.TrimPrefix(raw, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return nil, "", fmt.Errorf("missing frontmatter end")
	}
	front := rest[:idx]
	body := rest[idx+len("\n---\n"):]
	return []byte(strings.TrimSpace(front)), body, nil
}

func parseSections(body string) map[string][]string {
	lines := strings.Split(body, "\n")
	sections := map[string][]string{}
	current := ""
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			current = strings.TrimSpace(strings.TrimPrefix(trim, "## "))
			if _, ok := sections[current]; !ok {
				sections[current] = []string{}
			}
			continue
		}
		if current == "" || trim == "" {
			continue
		}
		sections[current] = append(sections[current], trim)
	}
	return sections
}

func firstLine(lines []string) string {
	for _, l := range lines {
		l = strings.TrimSpace(strings.TrimPrefix(l, "- "))
		if l != "" {
			return l
		}
	}
	return ""
}

func readEpisodeDoc(absPath string) (EpisodeDocument, error) {
	b, err := os.ReadFile(absPath)
	if err != nil {
		return EpisodeDocument{}, err
	}
	doc, err := parseEpisodeMarkdown(string(b))
	if err != nil {
		return EpisodeDocument{}, err
	}
	doc.Path = absPath
	return doc, nil
}

func readSnapshotDoc(absPath string) (SnapshotDocument, error) {
	b, err := os.ReadFile(absPath)
	if err != nil {
		return SnapshotDocument{}, err
	}
	doc, err := parseSnapshotMarkdown(string(b))
	if err != nil {
		return SnapshotDocument{}, err
	}
	doc.Path = absPath
	return doc, nil
}

func listSnapshotFiles(memoryRepoPath string) ([]string, error) {
	root := filepath.Join(memoryRepoPath, "snapshots")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func listEpisodeFiles(memoryRepoPath string) ([]string, error) {
	root := filepath.Join(memoryRepoPath, "episodes")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
