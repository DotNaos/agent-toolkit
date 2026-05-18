package handovercli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type BriefingOptions struct {
	Title           string
	SourceSession   string
	SourceProject   string
	TargetProject   string
	Mode            string
	Idea            string
	RequestedChange string
	Acceptance      []string
	Constraints     []string
	CreatedAt       time.Time
}

func BuildBriefing(opts BriefingOptions) string {
	created := opts.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	title := firstNonEmpty(opts.Title, "Codex Handover Briefing")
	mode := firstNonEmpty(opts.Mode, "worktree")
	sourceSession := firstNonEmpty(opts.SourceSession, "unknown")
	sourceProject := firstNonEmpty(opts.SourceProject, "unknown")
	targetProject := firstNonEmpty(opts.TargetProject, "unknown")
	idea := firstNonEmpty(opts.Idea, "Describe the idea to carry into the target project.")
	requestedChange := firstNonEmpty(opts.RequestedChange, "Implement the idea in the target project.")

	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "kind: codex-handover\n")
	fmt.Fprintf(&b, "version: 1\n")
	fmt.Fprintf(&b, "source_session: %q\n", sourceSession)
	fmt.Fprintf(&b, "source_project: %q\n", sourceProject)
	fmt.Fprintf(&b, "target_project: %q\n", targetProject)
	fmt.Fprintf(&b, "mode: %q\n", mode)
	fmt.Fprintf(&b, "created_at: %q\n", created.Format(time.RFC3339))
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "Hi Codex,\n\n")
	fmt.Fprintf(&b, "this is a handover from another Codex session: `%s`.\n\n", sourceSession)
	fmt.Fprintf(&b, "We worked in `%s` and found something that should be carried into `%s`.\n\n", sourceProject, targetProject)
	fmt.Fprintf(&b, "## Idea\n\n%s\n\n", idea)
	fmt.Fprintf(&b, "## Requested Change\n\n%s\n\n", requestedChange)
	fmt.Fprintf(&b, "## Implementation Mode\n\n`%s`\n\n", mode)
	writeList(&b, "Acceptance Criteria", opts.Acceptance, "The target project reflects the requested change and the result is verified.")
	writeList(&b, "Constraints", opts.Constraints, "Do not make unrelated changes.")
	fmt.Fprintf(&b, "## Suggested First Steps\n\n")
	fmt.Fprintf(&b, "1. Read this briefing completely.\n")
	fmt.Fprintf(&b, "2. Inspect the target project before editing.\n")
	fmt.Fprintf(&b, "3. Implement only the requested change using the selected mode.\n")
	fmt.Fprintf(&b, "4. Run focused verification and report what passed.\n")
	return b.String()
}

func DefaultBriefingPath(targetProject string, title string, now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	root := targetProject
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	slug := slugify(firstNonEmpty(title, "codex-handover"))
	name := fmt.Sprintf("%s-%s.md", now.Format("20060102-150405"), slug)
	return filepath.Join(root, ".codex", "handoffs", name)
}

func writeList(b *strings.Builder, title string, items []string, fallback string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			cleaned = append(cleaned, item)
		}
	}
	if len(cleaned) == 0 {
		cleaned = append(cleaned, fallback)
	}
	for _, item := range cleaned {
		fmt.Fprintf(b, "- %s\n", item)
	}
	fmt.Fprintf(b, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var slugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "codex-handover"
	}
	if len(value) > 64 {
		value = strings.Trim(value[:64], "-")
	}
	return value
}
