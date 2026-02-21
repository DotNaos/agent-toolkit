package chatd

import (
	"errors"
	"time"
)

const (
	DefaultListenAddr    = "127.0.0.1:45217"
	DefaultClientTimeout = 60 * time.Second
	MinWaitTimeout       = 1 * time.Second
	MaxWaitTimeout       = 15 * time.Minute
	DefaultLeaseDuration = 30 * time.Second
	DefaultPollInterval  = 200 * time.Millisecond
	DefaultAutoStartWait = 3 * time.Second
)

var (
	ErrMessageNotFound = errors.New("message not found")
	ErrLeaseConflict   = errors.New("message lease is invalid or expired")
)

type Message struct {
	ID        string  `json:"id"`
	ToAgent   string  `json:"to_agent"`
	FromAgent string  `json:"from_agent,omitempty"`
	ThreadID  string  `json:"thread_id,omitempty"`
	Body      string  `json:"body"`
	CreatedAt string  `json:"created_at"`
	Attempt   int64   `json:"attempt_count,omitempty"`
	AckedAt   *string `json:"acked_at,omitempty"`
}

type LeasedMessage struct {
	Message
	LeaseToken     string `json:"lease_token"`
	LeaseExpiresAt string `json:"lease_expires_at"`
}

type SendRequest struct {
	ToAgent   string `json:"to_agent"`
	FromAgent string `json:"from_agent,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	Body      string `json:"body"`
}

type SendResponse struct {
	Status    string `json:"status"`
	Action    string `json:"action"`
	MessageID string `json:"message_id"`
	ToAgent   string `json:"to_agent"`
	ThreadID  string `json:"thread_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

type WaitRequest struct {
	Agent    string `json:"agent"`
	ThreadID string `json:"thread_id,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
}

type WaitResponse struct {
	Status         string   `json:"status"`
	Action         string   `json:"action"`
	Message        *Message `json:"message,omitempty"`
	LeaseToken     string   `json:"lease_token,omitempty"`
	LeaseExpiresAt string   `json:"lease_expires_at,omitempty"`
	Agent          string   `json:"agent,omitempty"`
	Timeout        string   `json:"timeout,omitempty"`
}

type AckRequest struct {
	Agent      string `json:"agent"`
	MessageID  string `json:"message_id"`
	LeaseToken string `json:"lease_token"`
}

type AckResponse struct {
	Status  string `json:"status"`
	Action  string `json:"action"`
	ID      string `json:"id"`
	AckedAt string `json:"acked_at"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Action    string `json:"action"`
	Listen    string `json:"listen"`
	DBPath    string `json:"db_path"`
	StartedAt string `json:"started_at"`
}

type EnqueueParams struct {
	ToAgent   string
	FromAgent string
	ThreadID  string
	Body      string
}

type LeaseParams struct {
	Agent    string
	ThreadID string
}

type AckParams struct {
	Agent      string
	MessageID  string
	LeaseToken string
}

type Store interface {
	EnqueueMessage(params EnqueueParams) (*Message, error)
	LeaseNextMessage(params LeaseParams) (*LeasedMessage, error)
	AckMessage(params AckParams) (*time.Time, error)
	Close() error
}
