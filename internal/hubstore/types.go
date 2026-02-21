package hubstore

import "time"

const timestampFormat = "2006-01-02T15:04:05.000000000Z"

type ParticipantType string

const (
	ParticipantTypeHuman ParticipantType = "human"
	ParticipantTypeAgent ParticipantType = "agent"
)

type MessageKind string

const (
	MessageKindText   MessageKind = "text"
	MessageKindSystem MessageKind = "system"
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
	ApprovalStatusSelected ApprovalStatus = "selected"
	ApprovalStatusExpired  ApprovalStatus = "expired"
)

type DispatchStatus string

const (
	DispatchStatusQueued          DispatchStatus = "queued"
	DispatchStatusRunning         DispatchStatus = "running"
	DispatchStatusWaitingApproval DispatchStatus = "waiting_approval"
	DispatchStatusCompleted       DispatchStatus = "completed"
	DispatchStatusRejected        DispatchStatus = "rejected"
	DispatchStatusFailed          DispatchStatus = "failed"
)

type Conversation struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type Participant struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Type           ParticipantType `json:"type"`
	RefID          string          `json:"ref_id"`
	CreatedAt      string          `json:"created_at"`
}

type Message struct {
	ID             string      `json:"id"`
	ConversationID string      `json:"conversation_id"`
	FromID         string      `json:"from_id"`
	ToID           *string     `json:"to_id,omitempty"`
	Kind           MessageKind `json:"kind"`
	Body           string      `json:"body"`
	CreatedAt      string      `json:"created_at"`
}

type ApprovalRequest struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	AgentID        string         `json:"agent_id"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	SchemaJSON     string         `json:"schema_json"`
	RiskLevel      string         `json:"risk_level"`
	Status         ApprovalStatus `json:"status"`
	ExpiresAt      string         `json:"expires_at"`
	CreatedAt      string         `json:"created_at"`
	ResolvedAt     *string        `json:"resolved_at,omitempty"`
}

type ApprovalResponse struct {
	ID          string `json:"id"`
	ApprovalID  string `json:"approval_id"`
	HumanID     string `json:"human_id"`
	Decision    string `json:"decision"`
	PayloadJSON string `json:"payload_json"`
	CreatedAt   string `json:"created_at"`
}

type AgentDispatch struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	AgentID        string         `json:"agent_id"`
	Prompt         string         `json:"prompt"`
	MetadataJSON   string         `json:"metadata_json"`
	Status         DispatchStatus `json:"status"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type ParticipantInput struct {
	Type ParticipantType `json:"type"`
	ID   string          `json:"id"`
}

type CreateConversationParams struct {
	Name         string             `json:"name"`
	Participants []ParticipantInput `json:"participants"`
}

type AddMessageParams struct {
	ConversationID string
	FromID         string
	ToID           *string
	Kind           MessageKind
	Body           string
}

type CreateApprovalRequestParams struct {
	ConversationID string
	AgentID        string
	Title          string
	Description    string
	SchemaJSON     string
	RiskLevel      string
	ExpiresAt      time.Time
}

type RespondApprovalParams struct {
	ApprovalID  string
	HumanID     string
	Decision    string
	PayloadJSON string
}

type CreateDispatchParams struct {
	ConversationID string
	AgentID        string
	Prompt         string
	MetadataJSON   string
	Status         DispatchStatus
}

func nowTimestamp() string {
	return time.Now().UTC().Format(timestampFormat)
}

func NowTimestamp() string {
	return nowTimestamp()
}
