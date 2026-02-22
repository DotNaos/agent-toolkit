package memoryd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func defaultToolkitDirMemoryd() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agent-toolkit"
	}
	return filepath.Join(home, ".agent-toolkit")
}

func defaultMemoryReposRoot() string {
	return filepath.Join(defaultToolkitDirMemoryd(), "memory-repos")
}

func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func repoIDFromPath(repoPath string) string {
	clean := filepath.Clean(strings.TrimSpace(repoPath))
	sum := sha256.Sum256([]byte(clean))
	return "r_" + hex.EncodeToString(sum[:8])
}

func sanitizeBookmarkName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "task/unknown"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9._/-]+`)
	s = re.ReplaceAllString(s, "-")
	return strings.Trim(s, "-/")
}

func normalizeList(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return dedupeStrings(out)
}

func sortedKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	return keys
}
