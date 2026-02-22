package memorycli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agent-toolkit/internal/memoryd"
	"agent-toolkit/internal/shared/cliio"
)

const (
	envDBPath     = "AGENT_MEMORY_DB_PATH"
	envServerURL  = "AGENT_MEMORY_SERVER_URL"
	envOllamaURL  = "AGENT_MEMORY_OLLAMA_URL"
	envEmbedModel = "AGENT_MEMORY_EMBED_MODEL"
)

type Config struct {
	DBPath          string
	ListenAddr      string
	OllamaURL       string
	EmbeddingModel  string
	MemoryReposRoot string
}

func defaultToolkitDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agent-toolkit"
	}
	return filepath.Join(home, ".agent-toolkit")
}

func defaultDBPath() string { return filepath.Join(defaultToolkitDir(), "agent-memory.db") }

func expandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func resolveConfig(dbPathFlag string, listenFlag string, ollamaURLFlag string, embeddingModelFlag string, memoryReposRootFlag string) (Config, error) {
	dbPath := strings.TrimSpace(dbPathFlag)
	if dbPath == "" {
		dbPath = strings.TrimSpace(os.Getenv(envDBPath))
	}
	if dbPath == "" {
		dbPath = defaultDBPath()
	}
	resolvedDB, err := expandPath(dbPath)
	if err != nil {
		return Config{}, err
	}
	ollamaURL := strings.TrimSpace(ollamaURLFlag)
	if ollamaURL == "" {
		ollamaURL = strings.TrimSpace(os.Getenv(envOllamaURL))
	}
	if ollamaURL == "" {
		ollamaURL = memoryd.DefaultOllamaURL
	}
	model := strings.TrimSpace(embeddingModelFlag)
	if model == "" {
		model = strings.TrimSpace(os.Getenv(envEmbedModel))
	}
	if model == "" {
		model = memoryd.DefaultEmbeddingModel
	}
	listen := strings.TrimSpace(listenFlag)
	if listen == "" {
		listen = memoryd.DefaultListenAddr
	}
	memoryReposRoot := strings.TrimSpace(memoryReposRootFlag)
	if memoryReposRoot != "" {
		resolvedRoot, err := expandPath(memoryReposRoot)
		if err != nil {
			return Config{}, err
		}
		memoryReposRoot = resolvedRoot
	}
	return Config{DBPath: resolvedDB, ListenAddr: listen, OllamaURL: ollamaURL, EmbeddingModel: model, MemoryReposRoot: memoryReposRoot}, nil
}

func outputJSON(payload any) error     { return cliio.OutputJSON(payload) }
func FormatErrorJSON(err error) string { return cliio.FormatErrorJSON(err) }

func openStore(cfg Config) (*memoryd.Store, error) { return memoryd.NewStore(cfg.DBPath) }

func openRetriever(cfg Config) (*memoryd.Store, *memoryd.Retriever, error) {
	store, err := memoryd.NewStore(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	var embedder memoryd.Embedder
	if strings.TrimSpace(cfg.OllamaURL) != "-" {
		embedder = memoryd.NewOllamaEmbedder(cfg.OllamaURL, cfg.EmbeddingModel)
	}
	return store, memoryd.NewRetriever(store, embedder, memoryd.DefaultScoreThreshold), nil
}

func runPassthrough(cmdArgs []string) error {
	if len(cmdArgs) == 0 {
		return fmt.Errorf("missing command after --")
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
