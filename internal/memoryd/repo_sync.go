package memoryd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RepoSyncResult struct {
	RepoPath string   `json:"repo_path"`
	Files    []string `json:"files"`
	Added    int      `json:"added"`
}

func SyncRepoPreferences(store *Store, repoPath string) (*RepoSyncResult, error) {
	repoPath = filepath.Clean(strings.TrimSpace(repoPath))
	if repoPath == "" {
		return nil, fmt.Errorf("repo path is required")
	}
	files := []string{"AGENTS.md", "SKILL.md", "README.md"}
	entries := make([]UpsertMemoryParams, 0, 32)
	seenFiles := []string{}
	for _, name := range files {
		full := filepath.Join(repoPath, name)
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		seenFiles = append(seenFiles, full)
		lines, err := extractPreferenceLines(full)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			if ContainsLikelySecret(line) {
				continue
			}
			cat := classifyLine(line)
			if cat == "" {
				continue
			}
			entries = append(entries, UpsertMemoryParams{
				Scope:      ScopeRepo,
				RepoPath:   repoPath,
				Category:   cat,
				Title:      filepath.Base(full),
				Content:    line,
				Language:   detectLanguage(line),
				Tags:       deriveTags(line),
				SourceType: SourceRepoSync,
				Active:     true,
			})
		}
	}
	count, err := store.ReplaceRepoSyncMemories(repoPath, entries)
	if err != nil {
		return nil, err
	}
	return &RepoSyncResult{RepoPath: repoPath, Files: seenFiles, Added: count}, nil
}

func extractPreferenceLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "#") {
			continue
		}
		if !looksPreferenceLike(line) {
			continue
		}
		if len(line) > 400 {
			line = line[:400]
		}
		out = append(out, line)
	}
	return dedupeStrings(out), s.Err()
}

func looksPreferenceLike(line string) bool {
	l := strings.ToLower(line)
	keywords := []string{"always", "prefer", "must", "should", "never", "rule", "guideline", "bun", "uv", "pnpm", "npm", "yarn", "python", "frontend", "backend", "format", "lint", "test"}
	for _, k := range keywords {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

func classifyLine(line string) MemoryCategory {
	l := strings.ToLower(line)
	for _, k := range []string{"bun", "uv", "npm", "pnpm", "yarn", "poetry", "pip", "pipenv", "tool", "cli"} {
		if strings.Contains(l, k) {
			return CategoryTooling
		}
	}
	for _, k := range []string{"react", "next", "vue", "svelte", "django", "fastapi", "express", "tailwind", "framework"} {
		if strings.Contains(l, k) {
			return CategoryFramework
		}
	}
	for _, k := range []string{"always", "prefer", "must", "should", "never", "rule", "guideline", "review", "test", "format", "lint"} {
		if strings.Contains(l, k) {
			return CategoryCodingGuideline
		}
	}
	return ""
}

func deriveTags(line string) []string {
	terms := tokenize(line)
	keep := []string{}
	for _, t := range terms {
		if len(t) < 2 || len(t) > 24 {
			continue
		}
		keep = append(keep, t)
	}
	return keep
}

func detectLanguage(line string) string {
	for _, r := range line {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			break
		}
	}
	l := strings.ToLower(line)
	if strings.Contains(l, " der ") || strings.Contains(l, " und ") || strings.Contains(l, " nicht ") || strings.Contains(l, "immer") {
		return "de"
	}
	return "en"
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
