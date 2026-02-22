package memoryd

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

type Retriever struct {
	store          *Store
	embedder       Embedder
	scoreThreshold float64
}

func NewRetriever(store *Store, embedder Embedder, scoreThreshold float64) *Retriever {
	if scoreThreshold <= 0 {
		scoreThreshold = DefaultScoreThreshold
	}
	return &Retriever{store: store, embedder: embedder, scoreThreshold: scoreThreshold}
}

func (r *Retriever) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	if params.Limit <= 0 {
		params.Limit = DefaultTopK
	}
	if params.ScoreThreshold <= 0 {
		params.ScoreThreshold = r.scoreThreshold
	}
	params.RepoPath = cleanMaybePath(params.RepoPath)

	if strings.TrimSpace(params.Query) == "" {
		return SearchResponse{Results: nil}, nil
	}

	if r.embedder != nil {
		qvec, err := r.embedder.Embed(ctx, params.Query)
		if err == nil && len(qvec) > 0 {
			cands, err := r.store.LoadSearchCandidates(params.RepoPath, params.Categories)
			if err == nil {
				results := scoreCandidates(qvec, cands, params)
				if len(results) > 0 {
					return SearchResponse{Results: results, EmbeddingUsed: true}, nil
				}
				return SearchResponse{Results: nil, EmbeddingUsed: true}, nil
			}
		}
		fts, fErr := r.store.SearchFTS(params)
		if fErr != nil {
			return SearchResponse{FallbackUsed: true, EmbeddingUsed: false, Reason: "search failed"}, fErr
		}
		return SearchResponse{Results: filterByThreshold(fts, params.ScoreThreshold), FallbackUsed: true, EmbeddingUsed: false, Reason: "embedding unavailable"}, nil
	}

	fts, err := r.store.SearchFTS(params)
	if err != nil {
		return SearchResponse{}, err
	}
	return SearchResponse{Results: filterByThreshold(fts, params.ScoreThreshold), FallbackUsed: true, EmbeddingUsed: false, Reason: "embedder disabled"}, nil
}

func (r *Retriever) UpsertWithEmbedding(ctx context.Context, params UpsertMemoryParams) (*Memory, bool, error) {
	if ContainsLikelySecret(params.Content) || ContainsLikelySecret(params.Title) {
		return nil, false, fmt.Errorf("memory entry appears to contain a secret")
	}
	mem, err := r.store.UpsertMemory(params)
	if err != nil {
		return nil, false, err
	}
	if r.embedder == nil {
		return mem, false, nil
	}
	vec, err := r.embedder.Embed(ctx, mem.Title+"\n"+mem.Content+"\n"+strings.Join(mem.Tags, " "))
	if err != nil || len(vec) == 0 {
		return mem, false, nil
	}
	if err := r.store.SaveEmbedding(mem.ID, r.embedder.Model(), vec); err != nil {
		return mem, false, nil
	}
	return mem, true, nil
}

func scoreCandidates(qvec []float64, cands []EmbeddedMemory, params SearchParams) []SearchResult {
	terms := tokenize(params.Query)
	out := make([]SearchResult, 0, len(cands))
	for _, c := range cands {
		score := 0.0
		rankSource := "embedding"
		if len(c.Vector) > 0 {
			score = cosine(qvec, c.Vector)
		} else {
			rankSource = "embedding+fts-fallback"
			text := strings.ToLower(c.Title + "\n" + c.Content + "\n" + strings.Join(c.Tags, " "))
			for _, t := range terms {
				if strings.Contains(text, t) {
					score += 0.15
				}
			}
		}
		matchedTags := []string{}
		for _, tag := range c.Tags {
			for _, t := range terms {
				if strings.EqualFold(tag, t) {
					score += 0.08
					matchedTags = append(matchedTags, tag)
				}
			}
		}
		score += repoBoost(c.Memory, params.RepoPath)
		if score < params.ScoreThreshold {
			continue
		}
		out = append(out, SearchResult{Memory: c.Memory, Score: clampScore(score), RankSource: rankSource, MatchedTags: dedupeStrings(matchedTags)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if math.Abs(out[i].Score-out[j].Score) > 1e-9 {
			return out[i].Score > out[j].Score
		}
		if out[i].Memory.Scope != out[j].Memory.Scope {
			return out[i].Memory.Scope == ScopeRepo
		}
		return out[i].Memory.UpdatedAt > out[j].Memory.UpdatedAt
	})
	if len(out) > params.Limit {
		out = out[:params.Limit]
	}
	return out
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	dot := 0.0
	aNorm := 0.0
	bNorm := 0.0
	for i := range a {
		dot += a[i] * b[i]
		aNorm += a[i] * a[i]
		bNorm += b[i] * b[i]
	}
	if aNorm == 0 || bNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(aNorm) * math.Sqrt(bNorm))
}

func filterByThreshold(in []SearchResult, th float64) []SearchResult {
	if th <= 0 {
		return in
	}
	out := make([]SearchResult, 0, len(in))
	for _, r := range in {
		if r.Score >= th {
			out = append(out, r)
		}
	}
	return out
}

func cleanMaybePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Clean(path)
}

func BuildAgentHint(results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("User preference hint (local memory, repo-specific overrides first):\n")
	for i, r := range results {
		if i >= DefaultProxyHintsLimit {
			break
		}
		b.WriteString(fmt.Sprintf("- [%s/%s] %s\n", r.Memory.Scope, r.Memory.Category, strings.TrimSpace(r.Memory.Content)))
	}
	b.WriteString("Use this only if it does not conflict with explicit repo instructions in the current task.")
	return b.String()
}
