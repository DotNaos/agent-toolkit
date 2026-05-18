package handovercli

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type ThreadInfo struct {
	ID           string         `json:"id"`
	Title        string         `json:"title,omitempty"`
	CWD          string         `json:"cwd,omitempty"`
	RolloutPath  string         `json:"rollout_path,omitempty"`
	Source       string         `json:"source,omitempty"`
	ThreadSource sql.NullString `json:"-"`
	HasUserEvent bool           `json:"has_user_event"`
}

func DefaultStateDBPath() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "state_5.sqlite")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex", "state_5.sqlite")
	}
	return ""
}

func LookupThread(dbPath string, sessionID string) (*ThreadInfo, error) {
	if dbPath == "" {
		return nil, errors.New("missing Codex state DB path")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	row := db.QueryRow(`select id, title, cwd, rollout_path, source, thread_source, has_user_event from threads where id = ?`, sessionID)
	info := ThreadInfo{}
	if err := row.Scan(&info.ID, &info.Title, &info.CWD, &info.RolloutPath, &info.Source, &info.ThreadSource, &info.HasUserEvent); err != nil {
		return nil, err
	}
	return &info, nil
}

func LatestThreadForCWD(dbPath string, cwd string) (*ThreadInfo, error) {
	if dbPath == "" {
		return nil, errors.New("missing Codex state DB path")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	row := db.QueryRow(`select id, title, cwd, rollout_path, source, thread_source, has_user_event from threads where cwd = ? order by updated_at_ms desc limit 1`, cwd)
	info := ThreadInfo{}
	if err := row.Scan(&info.ID, &info.Title, &info.CWD, &info.RolloutPath, &info.Source, &info.ThreadSource, &info.HasUserEvent); err != nil {
		return nil, err
	}
	return &info, nil
}
