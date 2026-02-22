package memorycli

import (
	"context"
	"fmt"
	"strings"

	"agent-toolkit/internal/memoryd"
	"github.com/spf13/cobra"
)

var (
	memoryScope      string
	memoryRepoPath   string
	memoryCategory   string
	memoryTitle      string
	memoryContent    string
	memoryLanguage   string
	memoryTags       string
	memorySourceType string
	memoryID         string
	memoryQuery      string
	memoryLimit      int
	memoryThreshold  float64
)

var memoryCmd = &cobra.Command{Use: "memory", Short: "Manage local memory entries"}

var memoryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add or update a memory entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, retriever, err := openRetriever(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		mem, embedded, err := retriever.UpsertWithEmbedding(context.Background(), memoryd.UpsertMemoryParams{
			ID:         memoryID,
			Scope:      memoryd.MemoryScope(memoryScope),
			RepoPath:   memoryRepoPath,
			Category:   memoryd.MemoryCategory(memoryCategory),
			Title:      memoryTitle,
			Content:    memoryContent,
			Language:   memoryLanguage,
			Tags:       parseCSV(memoryTags),
			SourceType: memoryd.MemorySourceType(memorySourceType),
			Active:     true,
		})
		if err != nil {
			return err
		}
		return outputJSON(map[string]any{"status": "success", "action": "memory.add", "memory": mem, "embedding_saved": embedded})
	},
}

var memoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memory entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		items, err := store.ListMemories(memoryScope, memoryRepoPath, memoryLimit)
		if err != nil {
			return err
		}
		return outputJSON(map[string]any{"status": "success", "action": "memory.list", "memories": items})
	},
}

var memorySearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search memory entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(memoryQuery) == "" {
			return fmt.Errorf("--query is required")
		}
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, retriever, err := openRetriever(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		resp, err := retriever.Search(context.Background(), memoryd.SearchParams{
			Query:          memoryQuery,
			RepoPath:       memoryRepoPath,
			Categories:     parseCategories(memoryCategory),
			Limit:          memoryLimit,
			ScoreThreshold: memoryThreshold,
		})
		if err != nil {
			return err
		}
		return outputJSON(map[string]any{"status": "success", "action": "memory.search", "search": resp})
	},
}

var memoryRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Delete a memory entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(memoryID) == "" {
			return fmt.Errorf("--id is required")
		}
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.DeleteMemory(memoryID); err != nil {
			return err
		}
		return outputJSON(map[string]any{"status": "success", "action": "memory.remove", "id": memoryID})
	},
}

var memorySeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed default bun/uv preferences",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, retriever, err := openRetriever(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		seed := []memoryd.UpsertMemoryParams{
			{Scope: memoryd.ScopeGlobal, Category: memoryd.CategoryTooling, Title: "Frontend tooling", Content: "Use bun by default for frontend work unless repo conventions say otherwise.", Language: "en", Tags: []string{"bun", "frontend"}, SourceType: memoryd.SourceSeed, Active: true},
			{Scope: memoryd.ScopeGlobal, Category: memoryd.CategoryTooling, Title: "Python tooling", Content: "Use uv by default for Python work unless repo conventions say otherwise.", Language: "en", Tags: []string{"uv", "python"}, SourceType: memoryd.SourceSeed, Active: true},
		}
		added := 0
		for _, p := range seed {
			if _, _, err := retriever.UpsertWithEmbedding(context.Background(), p); err == nil {
				added++
			}
		}
		return outputJSON(map[string]any{"status": "success", "action": "memory.seed", "added": added})
	},
}

func init() {
	memoryCmd.PersistentFlags().StringVar(&memoryScope, "scope", string(memoryd.ScopeGlobal), "Memory scope: global|repo")
	memoryCmd.PersistentFlags().StringVar(&memoryRepoPath, "repo-path", "", "Repo path for repo-scoped operations")
	memoryCmd.PersistentFlags().StringVar(&memoryCategory, "category", string(memoryd.CategoryTooling), "Category (or CSV for search)")
	memoryCmd.PersistentFlags().StringVar(&memoryLanguage, "language", "", "Language tag (de|en)")
	memoryCmd.PersistentFlags().StringVar(&memoryTags, "tags", "", "Comma-separated tags")
	memoryCmd.PersistentFlags().StringVar(&memorySourceType, "source-type", string(memoryd.SourceManual), "Source type")
	memoryCmd.PersistentFlags().StringVar(&memoryID, "id", "", "Memory ID (for update/remove)")
	memoryCmd.PersistentFlags().IntVar(&memoryLimit, "limit", 20, "Result limit")
	memoryCmd.PersistentFlags().Float64Var(&memoryThreshold, "score-threshold", 0.0, "Search score threshold override")

	memoryAddCmd.Flags().StringVar(&memoryTitle, "title", "", "Title")
	memoryAddCmd.Flags().StringVar(&memoryContent, "content", "", "Content")
	_ = memoryAddCmd.MarkFlagRequired("title")
	_ = memoryAddCmd.MarkFlagRequired("content")

	memorySearchCmd.Flags().StringVar(&memoryQuery, "query", "", "Search query")
	_ = memorySearchCmd.MarkFlagRequired("query")

	memoryCmd.AddCommand(memoryAddCmd, memoryListCmd, memorySearchCmd, memoryRemoveCmd, memorySeedCmd)
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseCategories(raw string) []memoryd.MemoryCategory {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := parseCSV(raw)
	out := make([]memoryd.MemoryCategory, 0, len(parts))
	for _, p := range parts {
		out = append(out, memoryd.MemoryCategory(p))
	}
	return out
}
