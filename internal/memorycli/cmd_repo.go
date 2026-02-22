package memorycli

import (
	"os"

	"agent-toolkit/internal/memoryd"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{Use: "repo", Short: "Repo-specific memory sync"}

var repoSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync AGENTS.md/SKILL.md/README.md preferences into repo-scoped memory",
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
		repoPath := memoryRepoPath
		if repoPath == "" {
			repoPath, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		res, err := memoryd.SyncRepoPreferences(store, repoPath)
		if err != nil {
			return err
		}
		return outputJSON(map[string]any{"status": "success", "action": "repo.sync", "result": res})
	},
}

func init() {
	repoSyncCmd.Flags().StringVar(&memoryRepoPath, "repo-path", "", "Repository path (default cwd)")
	repoCmd.AddCommand(repoSyncCmd)
}
