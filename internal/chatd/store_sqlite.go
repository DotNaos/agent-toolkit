package chatd

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

const timestampFormat = "2006-01-02T15:04:05.000000000Z"

type SQLiteStore struct {
	db            *sql.DB
	leaseDuration time.Duration
}

func NewSQLiteStore(path string, leaseDuration time.Duration) (*SQLiteStore, error) {
	if leaseDuration <= 0 {
		leaseDuration = DefaultLeaseDuration
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &SQLiteStore{db: db, leaseDuration: leaseDuration}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	stmts := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			to_agent TEXT NOT NULL,
			from_agent TEXT,
			thread_id TEXT,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL,
			lease_token TEXT,
			leased_by TEXT,
			lease_until TEXT,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			acked_at TEXT
		);`,
		"CREATE INDEX IF NOT EXISTS idx_messages_delivery ON messages(to_agent, acked_at, lease_until, created_at, id);",
		"CREATE INDEX IF NOT EXISTS idx_messages_thread_delivery ON messages(thread_id, to_agent, acked_at, lease_until, created_at, id);",
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) EnqueueMessage(params EnqueueParams) (*Message, error) {
	if strings.TrimSpace(params.ToAgent) == "" {
		return nil, fmt.Errorf("to_agent is required")
	}
	if strings.TrimSpace(params.Body) == "" {
		return nil, fmt.Errorf("body is required")
	}

	id := ulid.Make().String()
	createdAt := nowTimestamp()

	_, err := s.db.Exec(
		`INSERT INTO messages (id, to_agent, from_agent, thread_id, body, created_at)
		 VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		id,
		params.ToAgent,
		params.FromAgent,
		params.ThreadID,
		params.Body,
		createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert message: %w", err)
	}

	return &Message{
		ID:        id,
		ToAgent:   params.ToAgent,
		FromAgent: params.FromAgent,
		ThreadID:  params.ThreadID,
		Body:      params.Body,
		CreatedAt: createdAt,
	}, nil
}

func (s *SQLiteStore) LeaseNextMessage(params LeaseParams) (*LeasedMessage, error) {
	if strings.TrimSpace(params.Agent) == "" {
		return nil, fmt.Errorf("agent is required")
	}

	now := nowTimestamp()
	leaseUntil := time.Now().UTC().Add(s.leaseDuration).Format(timestampFormat)
	leaseToken, err := randomToken(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate lease token: %w", err)
	}

	query := `WITH candidate AS (
		SELECT id
		FROM messages
		WHERE to_agent = ?
		  AND acked_at IS NULL
		  AND (lease_until IS NULL OR lease_until <= ?)`
	args := []any{params.Agent, now}
	if params.ThreadID != "" {
		query += "\n  AND thread_id = ?"
		args = append(args, params.ThreadID)
	}
	query += `
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	)
	UPDATE messages
	SET lease_token = ?, leased_by = ?, lease_until = ?, attempt_count = attempt_count + 1
	WHERE id = (SELECT id FROM candidate)
	RETURNING id, to_agent, from_agent, thread_id, body, created_at, lease_token, lease_until, attempt_count;`

	args = append(args, leaseToken, params.Agent, leaseUntil)

	var (
		msg       LeasedMessage
		fromAgent sql.NullString
		threadID  sql.NullString
	)

	err = s.db.QueryRow(query, args...).Scan(
		&msg.ID,
		&msg.ToAgent,
		&fromAgent,
		&threadID,
		&msg.Body,
		&msg.CreatedAt,
		&msg.LeaseToken,
		&msg.LeaseExpiresAt,
		&msg.Attempt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lease next message: %w", err)
	}

	if fromAgent.Valid {
		msg.FromAgent = fromAgent.String
	}
	if threadID.Valid {
		msg.ThreadID = threadID.String
	}

	return &msg, nil
}

func (s *SQLiteStore) AckMessage(params AckParams) (*time.Time, error) {
	if strings.TrimSpace(params.Agent) == "" {
		return nil, fmt.Errorf("agent is required")
	}
	if strings.TrimSpace(params.MessageID) == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if strings.TrimSpace(params.LeaseToken) == "" {
		return nil, fmt.Errorf("lease_token is required")
	}

	ackedAt := nowTimestamp()
	now := nowTimestamp()

	res, err := s.db.Exec(
		`UPDATE messages
		 SET acked_at = ?, lease_token = NULL, leased_by = NULL, lease_until = NULL
		 WHERE id = ?
		   AND leased_by = ?
		   AND lease_token = ?
		   AND acked_at IS NULL
		   AND lease_until > ?`,
		ackedAt,
		params.MessageID,
		params.Agent,
		params.LeaseToken,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to ack message: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to read ack result: %w", err)
	}
	if rows == 0 {
		exists, err := s.messageExists(params.MessageID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrMessageNotFound
		}
		return nil, ErrLeaseConflict
	}

	parsedAck, err := time.Parse(timestampFormat, ackedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ack timestamp: %w", err)
	}

	return &parsedAck, nil
}

func (s *SQLiteStore) messageExists(messageID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM messages WHERE id = ? LIMIT 1`, messageID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check message existence: %w", err)
	}
	return true, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func nowTimestamp() string {
	return time.Now().UTC().Format(timestampFormat)
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
