package memoryd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type JJManager struct {
	memoryReposRoot string
}

func NewJJManager(root string) *JJManager {
	if strings.TrimSpace(root) == "" {
		root = defaultMemoryReposRoot()
	}
	return &JJManager{memoryReposRoot: root}
}

func (m *JJManager) MemoryReposRoot() string { return m.memoryReposRoot }

func (m *JJManager) ResolveBinding(repoPath string) (RepoBinding, error) {
	repoPath = filepath.Clean(strings.TrimSpace(repoPath))
	if repoPath == "" {
		return RepoBinding{}, fmt.Errorf("repo_path is required")
	}
	id := repoIDFromPath(repoPath)
	memRepoPath := filepath.Join(m.memoryReposRoot, id)
	return RepoBinding{RepoID: id, RepoPath: repoPath, MemoryRepoPath: memRepoPath}, nil
}

func (m *JJManager) EnsureRepo(binding RepoBinding) error {
	if err := os.MkdirAll(binding.MemoryRepoPath, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(binding.MemoryRepoPath, ".jj")); err == nil {
		return m.ensureCanonicalLayout(binding)
	}
	if _, _, err := runCommand("", "jj", "git", "init", binding.MemoryRepoPath); err != nil {
		return fmt.Errorf("jj git init failed: %w", err)
	}
	if err := m.ensureCanonicalLayout(binding); err != nil {
		return err
	}
	// Create an initial tracked file so the repo has a meaningful main bookmark target.
	policyFile := filepath.Join(binding.MemoryRepoPath, "policy", "repo-scope.json")
	if _, err := os.Stat(policyFile); err != nil {
		content := []byte(fmt.Sprintf("{\n  \"repo_id\": %q,\n  \"repo_path\": %q\n}\n", binding.RepoID, binding.RepoPath))
		if err := os.WriteFile(policyFile, content, 0o644); err != nil {
			return err
		}
		_, _, _ = m.runJJ(binding, "commit", "-m", "init memory repo", "policy/repo-scope.json")
	}
	_, _, _ = m.runJJ(binding, "bookmark", "set", "main", "-r", "@-")
	return nil
}

func (m *JJManager) ensureCanonicalLayout(binding RepoBinding) error {
	dirs := []string{"episodes", "snapshots", "digests", "manifests/targets", "policy"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(binding.MemoryRepoPath, d), 0o755); err != nil {
			return err
		}
	}
	policyFile := filepath.Join(binding.MemoryRepoPath, "policy", "repo-scope.json")
	if _, err := os.Stat(policyFile); err != nil {
		content := []byte(fmt.Sprintf("{\n  \"repo_id\": %q,\n  \"repo_path\": %q\n}\n", binding.RepoID, binding.RepoPath))
		if err := os.WriteFile(policyFile, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *JJManager) TaskStart(binding RepoBinding, taskID string) (TaskRecord, error) {
	if err := m.EnsureRepo(binding); err != nil {
		return TaskRecord{}, err
	}
	bookmark := sanitizeBookmarkName("task/" + taskID)
	if _, _, err := m.runJJ(binding, "new", "main"); err != nil {
		// fallback in case main is not available yet
		if _, _, err2 := m.runJJ(binding, "new", "@"); err2 != nil {
			return TaskRecord{}, err
		}
	}
	_, _, _ = m.runJJ(binding, "bookmark", "set", bookmark, "-r", "@")
	return TaskRecord{
		TaskID:         taskID,
		RepoID:         binding.RepoID,
		RepoPath:       binding.RepoPath,
		MemoryRepoPath: binding.MemoryRepoPath,
		Status:         "running",
		Bookmark:       bookmark,
		StartedAt:      nowRFC3339Nano(),
	}, nil
}

func (m *JJManager) CommitFileAtom(binding RepoBinding, relativePath string, message string) (string, string, error) {
	relativePath = filepath.ToSlash(strings.TrimPrefix(filepath.Clean(relativePath), filepath.Clean(binding.MemoryRepoPath)))
	relativePath = strings.TrimPrefix(relativePath, "/")
	if relativePath == "" {
		return "", "", fmt.Errorf("relativePath is required")
	}
	if strings.TrimSpace(message) == "" {
		message = "memory atom"
	}
	if _, _, err := m.runJJ(binding, "commit", "-m", message, relativePath); err != nil {
		return "", "", err
	}
	// Best effort parse of last committed change IDs.
	stdout, _, err := m.runJJ(binding, "log", "-r", "@-", "--no-graph", "-n", "1", "-T", "commit_id.short(12) ++ \"|\" ++ change_id.short(12)")
	if err != nil {
		return "", "", nil
	}
	parts := strings.Split(strings.TrimSpace(stdout), "|")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0]), nil
	}
	return "", "", nil
}

func (m *JJManager) BookmarkSet(binding RepoBinding, name, rev string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("bookmark name is required")
	}
	if strings.TrimSpace(rev) == "" {
		rev = "@"
	}
	_, _, err := m.runJJ(binding, "bookmark", "set", name, "-r", rev)
	return err
}

func (m *JJManager) Abandon(binding RepoBinding, rev string) error {
	args := []string{"abandon"}
	if strings.TrimSpace(rev) != "" {
		args = append(args, rev)
	}
	_, _, err := m.runJJ(binding, args...)
	return err
}

func (m *JJManager) runJJ(binding RepoBinding, args ...string) (string, string, error) {
	return runCommand(binding.MemoryRepoPath, "jj", args...)
}

func runCommand(cwd string, bin string, args ...string) (string, string, error) {
	cmd := exec.Command(bin, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(),
		defaultIfMissingEnv("JJ_USER", "agent-memory"),
		defaultIfMissingEnv("JJ_EMAIL", "agent-memory@local.invalid"),
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return outBuf.String(), errBuf.String(), fmt.Errorf("%s %s failed: %s", bin, strings.Join(args, " "), msg)
	}
	return outBuf.String(), errBuf.String(), nil
}

func defaultIfMissingEnv(key, value string) string {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, key+"=") {
			return kv
		}
	}
	return key + "=" + value
}
