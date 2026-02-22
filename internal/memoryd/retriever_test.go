package memoryd

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestRetriever(t *testing.T) (*Store, *Retriever) {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, NewRetriever(store, nil, 0.2)
}

func TestRetrieverRepoScopedOverridesGlobal(t *testing.T) {
	store, retriever := newTestRetriever(t)
	_, _, err := retriever.UpsertWithEmbedding(context.Background(), UpsertMemoryParams{
		Scope: ScopeGlobal, Category: CategoryTooling, Title: "Global npm", Content: "Use npm for JS projects.", SourceType: SourceManual, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	repoPath := "/Users/oli/projects/active/agent-toolkit"
	_, err = store.UpsertMemory(UpsertMemoryParams{
		Scope: ScopeRepo, RepoPath: repoPath, Category: CategoryTooling, Title: "Repo bun", Content: "Use bun for frontend tooling in this repo.", SourceType: SourceRepoSync, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := retriever.Search(context.Background(), SearchParams{Query: "frontend package manager bun npm", RepoPath: repoPath, Limit: 5, ScoreThreshold: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) == 0 {
		t.Fatalf("expected search results")
	}
	if resp.Results[0].Memory.Scope != ScopeRepo {
		t.Fatalf("expected repo-scoped result first, got %+v", resp.Results[0])
	}
}

func TestRetrieverBlocksLikelySecrets(t *testing.T) {
	_, retriever := newTestRetriever(t)
	_, _, err := retriever.UpsertWithEmbedding(context.Background(), UpsertMemoryParams{
		Scope: ScopeGlobal, Category: CategoryCodingGuideline, Title: "secret", Content: "api_key=sk-1234567890abcdef", SourceType: SourceManual, Active: true,
	})
	if err == nil {
		t.Fatalf("expected secret detection error")
	}
}
