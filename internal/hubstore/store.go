package hubstore

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidState    = errors.New("invalid state transition")
	ErrAlreadyResolved = errors.New("already resolved")
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db path is required")
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

	s := &Store{db: db}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initSchema() error {
	stmts := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS participants (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			type TEXT NOT NULL,
			ref_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(conversation_id) REFERENCES conversations(id)
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			from_id TEXT NOT NULL,
			to_id TEXT,
			kind TEXT NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(conversation_id) REFERENCES conversations(id)
		);`,
		`CREATE TABLE IF NOT EXISTS approval_requests (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			schema_json TEXT NOT NULL,
			risk_level TEXT NOT NULL,
			status TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			resolved_at TEXT,
			FOREIGN KEY(conversation_id) REFERENCES conversations(id)
		);`,
		`CREATE TABLE IF NOT EXISTS approval_responses (
			id TEXT PRIMARY KEY,
			approval_id TEXT NOT NULL,
			human_id TEXT NOT NULL,
			decision TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(approval_id) REFERENCES approval_requests(id)
		);`,
		`CREATE TABLE IF NOT EXISTS agent_dispatch (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			prompt TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(conversation_id) REFERENCES conversations(id)
		);`,
		"CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, created_at, id);",
		"CREATE INDEX IF NOT EXISTS idx_approval_requests_pending ON approval_requests(conversation_id, status, created_at);",
		"CREATE INDEX IF NOT EXISTS idx_agent_dispatch_status ON agent_dispatch(agent_id, status, updated_at);",
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("schema init failed: %w", err)
		}
	}

	return nil
}

func (s *Store) CreateConversation(params CreateConversationParams) (*Conversation, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	conversation := &Conversation{ID: ulid.Make().String(), Name: strings.TrimSpace(params.Name), CreatedAt: nowTimestamp()}
	if _, err := tx.Exec(`INSERT INTO conversations(id, name, created_at) VALUES (?, ?, ?)`, conversation.ID, conversation.Name, conversation.CreatedAt); err != nil {
		return nil, err
	}

	for _, p := range params.Participants {
		if p.Type != ParticipantTypeHuman && p.Type != ParticipantTypeAgent {
			return nil, fmt.Errorf("invalid participant type %q", p.Type)
		}
		if strings.TrimSpace(p.ID) == "" {
			return nil, fmt.Errorf("participant id is required")
		}
		if _, err := tx.Exec(`INSERT INTO participants(id, conversation_id, type, ref_id, created_at) VALUES (?, ?, ?, ?, ?)`, ulid.Make().String(), conversation.ID, p.Type, strings.TrimSpace(p.ID), nowTimestamp()); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return conversation, nil
}

func (s *Store) ListConversations(limit int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, name, created_at FROM conversations ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Conversation, 0, limit)
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (s *Store) AddMessage(params AddMessageParams) (*Message, error) {
	if strings.TrimSpace(params.ConversationID) == "" {
		return nil, fmt.Errorf("conversation_id is required")
	}
	if strings.TrimSpace(params.FromID) == "" {
		return nil, fmt.Errorf("from_id is required")
	}
	if strings.TrimSpace(params.Body) == "" {
		return nil, fmt.Errorf("body is required")
	}
	if params.Kind == "" {
		params.Kind = MessageKindText
	}
	if params.Kind != MessageKindText && params.Kind != MessageKindSystem {
		return nil, fmt.Errorf("invalid kind %q", params.Kind)
	}
	if ok, err := s.conversationExists(params.ConversationID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNotFound
	}

	msg := &Message{
		ID:             ulid.Make().String(),
		ConversationID: params.ConversationID,
		FromID:         strings.TrimSpace(params.FromID),
		ToID:           params.ToID,
		Kind:           params.Kind,
		Body:           params.Body,
		CreatedAt:      nowTimestamp(),
	}

	_, err := s.db.Exec(`INSERT INTO messages(id, conversation_id, from_id, to_id, kind, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ConversationID, msg.FromID, msg.ToID, msg.Kind, msg.Body, msg.CreatedAt)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *Store) ListMessages(conversationID, cursor string, limit int) ([]Message, string, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, "", fmt.Errorf("conversation_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := `SELECT id, conversation_id, from_id, to_id, kind, body, created_at
		FROM messages
		WHERE conversation_id = ?`
	args := []any{conversationID}
	if strings.TrimSpace(cursor) != "" {
		parts := strings.SplitN(cursor, "|", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		query += ` AND (created_at > ? OR (created_at = ? AND id > ?))`
		args = append(args, parts[0], parts[0], parts[1])
	}
	query += ` ORDER BY created_at ASC, id ASC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	messages := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		var toID sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.FromID, &toID, &m.Kind, &m.Body, &m.CreatedAt); err != nil {
			return nil, "", err
		}
		if toID.Valid {
			m.ToID = &toID.String
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(messages) > limit {
		last := messages[limit-1]
		nextCursor = last.CreatedAt + "|" + last.ID
		messages = messages[:limit]
	}

	return messages, nextCursor, nil
}

func (s *Store) CreateApprovalRequest(params CreateApprovalRequestParams) (*ApprovalRequest, error) {
	if strings.TrimSpace(params.ConversationID) == "" || strings.TrimSpace(params.AgentID) == "" {
		return nil, fmt.Errorf("conversation_id and agent_id are required")
	}
	if strings.TrimSpace(params.Title) == "" || strings.TrimSpace(params.Description) == "" {
		return nil, fmt.Errorf("title and description are required")
	}
	if strings.TrimSpace(params.SchemaJSON) == "" {
		return nil, fmt.Errorf("schema is required")
	}
	if params.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("expires_at is required")
	}

	item := &ApprovalRequest{
		ID:             ulid.Make().String(),
		ConversationID: strings.TrimSpace(params.ConversationID),
		AgentID:        strings.TrimSpace(params.AgentID),
		Title:          strings.TrimSpace(params.Title),
		Description:    strings.TrimSpace(params.Description),
		SchemaJSON:     params.SchemaJSON,
		RiskLevel:      strings.TrimSpace(params.RiskLevel),
		Status:         ApprovalStatusPending,
		ExpiresAt:      params.ExpiresAt.UTC().Format(timestampFormat),
		CreatedAt:      nowTimestamp(),
	}
	if item.RiskLevel == "" {
		item.RiskLevel = "high"
	}

	_, err := s.db.Exec(`INSERT INTO approval_requests(id, conversation_id, agent_id, title, description, schema_json, risk_level, status, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.ConversationID, item.AgentID, item.Title, item.Description, item.SchemaJSON, item.RiskLevel, item.Status, item.ExpiresAt, item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) GetApprovalRequest(id string) (*ApprovalRequest, error) {
	var item ApprovalRequest
	var resolvedAt sql.NullString
	err := s.db.QueryRow(`SELECT id, conversation_id, agent_id, title, description, schema_json, risk_level, status, expires_at, created_at, resolved_at
		FROM approval_requests WHERE id = ?`, id).Scan(
		&item.ID, &item.ConversationID, &item.AgentID, &item.Title, &item.Description, &item.SchemaJSON, &item.RiskLevel, &item.Status, &item.ExpiresAt, &item.CreatedAt, &resolvedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if resolvedAt.Valid {
		item.ResolvedAt = &resolvedAt.String
	}
	return &item, nil
}

func (s *Store) MarkExpiredApprovals(now time.Time) (int64, error) {
	res, err := s.db.Exec(`UPDATE approval_requests
		SET status = ?, resolved_at = ?
		WHERE status = ? AND expires_at <= ?`,
		ApprovalStatusExpired,
		now.UTC().Format(timestampFormat),
		ApprovalStatusPending,
		now.UTC().Format(timestampFormat),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RespondApproval(params RespondApprovalParams) (*ApprovalRequest, error) {
	if strings.TrimSpace(params.ApprovalID) == "" {
		return nil, fmt.Errorf("approval_id is required")
	}
	if strings.TrimSpace(params.HumanID) == "" {
		return nil, fmt.Errorf("human_id is required")
	}
	decision := strings.TrimSpace(strings.ToLower(params.Decision))
	if decision != "approve" && decision != "reject" && decision != "select" {
		return nil, fmt.Errorf("invalid decision")
	}
	if strings.TrimSpace(params.PayloadJSON) == "" {
		params.PayloadJSON = "{}"
	}

	current, err := s.GetApprovalRequest(params.ApprovalID)
	if err != nil {
		return nil, err
	}
	if current.Status != ApprovalStatusPending {
		return nil, ErrAlreadyResolved
	}

	now := time.Now().UTC()
	expiresAt, err := time.Parse(timestampFormat, current.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if !expiresAt.After(now) {
		if _, err := s.db.Exec(`UPDATE approval_requests SET status = ?, resolved_at = ? WHERE id = ?`, ApprovalStatusExpired, now.Format(timestampFormat), current.ID); err != nil {
			return nil, err
		}
		return nil, ErrInvalidState
	}

	mappedStatus := ApprovalStatusRejected
	switch decision {
	case "approve":
		mappedStatus = ApprovalStatusApproved
	case "reject":
		mappedStatus = ApprovalStatusRejected
	case "select":
		mappedStatus = ApprovalStatusSelected
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	resolvedAt := now.Format(timestampFormat)
	result, err := tx.Exec(`UPDATE approval_requests SET status = ?, resolved_at = ? WHERE id = ? AND status = ?`, mappedStatus, resolvedAt, current.ID, ApprovalStatusPending)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrAlreadyResolved
	}

	if _, err := tx.Exec(`INSERT INTO approval_responses(id, approval_id, human_id, decision, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, ulid.Make().String(), current.ID, strings.TrimSpace(params.HumanID), decision, params.PayloadJSON, now.Format(timestampFormat)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	updated := *current
	updated.Status = mappedStatus
	updated.ResolvedAt = &resolvedAt
	return &updated, nil
}

func (s *Store) ListPendingApprovals(conversationID string, limit int) ([]ApprovalRequest, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversation_id is required")
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`SELECT id, conversation_id, agent_id, title, description, schema_json, risk_level, status, expires_at, created_at, resolved_at
		FROM approval_requests
		WHERE conversation_id = ? AND status = ?
		ORDER BY created_at ASC
		LIMIT ?`, conversationID, ApprovalStatusPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ApprovalRequest, 0, limit)
	for rows.Next() {
		var item ApprovalRequest
		var resolvedAt sql.NullString
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.AgentID, &item.Title, &item.Description, &item.SchemaJSON, &item.RiskLevel, &item.Status, &item.ExpiresAt, &item.CreatedAt, &resolvedAt); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateDispatch(params CreateDispatchParams) (*AgentDispatch, error) {
	if strings.TrimSpace(params.ConversationID) == "" || strings.TrimSpace(params.AgentID) == "" {
		return nil, fmt.Errorf("conversation_id and agent_id are required")
	}
	if strings.TrimSpace(params.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if params.Status == "" {
		params.Status = DispatchStatusQueued
	}
	if strings.TrimSpace(params.MetadataJSON) == "" {
		params.MetadataJSON = "{}"
	}

	now := nowTimestamp()
	item := &AgentDispatch{
		ID:             ulid.Make().String(),
		ConversationID: strings.TrimSpace(params.ConversationID),
		AgentID:        strings.TrimSpace(params.AgentID),
		Prompt:         params.Prompt,
		MetadataJSON:   params.MetadataJSON,
		Status:         params.Status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := s.db.Exec(`INSERT INTO agent_dispatch(id, conversation_id, agent_id, prompt, metadata_json, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.ConversationID, item.AgentID, item.Prompt, item.MetadataJSON, item.Status, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) UpdateDispatchStatus(id string, status DispatchStatus) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id is required")
	}
	res, err := s.db.Exec(`UPDATE agent_dispatch SET status = ?, updated_at = ? WHERE id = ?`, status, nowTimestamp(), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) conversationExists(conversationID string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM conversations WHERE id = ? LIMIT 1`, conversationID).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
