package memorycli

import (
	"agent-toolkit/internal/memoryd"
	"github.com/spf13/cobra"
)

var (
	flagDBPath          string
	flagListen          string
	flagOllamaURL       string
	flagEmbeddingModel  string
	flagMemoryReposRoot string
)

var rootCmd = &cobra.Command{
	Use:           "agent-memory",
	Short:         "Local agent preference memory store and proxy helper",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagDBPath, "db", "", "Path to agent-memory sqlite database (default ~/.agent-toolkit/agent-memory.db)")
	rootCmd.PersistentFlags().StringVar(&flagListen, "listen", memoryd.DefaultListenAddr, "Daemon listen address")
	rootCmd.PersistentFlags().StringVar(&flagOllamaURL, "ollama-url", "", "Ollama base URL (use '-' to disable embeddings)")
	rootCmd.PersistentFlags().StringVar(&flagEmbeddingModel, "embedding-model", "", "Ollama embedding model")
	rootCmd.PersistentFlags().StringVar(&flagMemoryReposRoot, "memory-repos-root", "", "Root directory for jj memory repos (default ~/.agent-toolkit/memory-repos)")

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(memoryCmd)
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(compatCmd)
	rootCmd.AddCommand(launchCmd)
}

func Execute() error { return rootCmd.Execute() }
