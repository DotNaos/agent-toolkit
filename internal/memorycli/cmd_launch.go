package memorycli

import (
	"os"

	"agent-toolkit/internal/memoryd"
	"github.com/spf13/cobra"
)

var launchCmd = &cobra.Command{
	Use:                "launch -- <command> [args...]",
	Short:              "Optional wrapper for tools that cannot be MITM-injected",
	DisableFlagParsing: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err == nil {
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				_, _ = memoryd.SyncRepoPreferences(store, cwd)
			}
			_ = store.Close()
		}
		return runPassthrough(args)
	},
}
