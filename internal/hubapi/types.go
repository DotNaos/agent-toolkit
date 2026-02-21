package hubapi

import "encoding/json"

type CreateConversationRequest struct {
	Name         string `json:"name"`
	Participants []struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"participants"`
}

type CreateConversationResponse struct {
	ConversationID string `json:"conversation_id"`
	CreatedAt      string `json:"created_at"`
}

type PostMessageRequest struct {
	ConversationID string  `json:"conversation_id"`
	FromID         string  `json:"from_id"`
	ToID           *string `json:"to_id"`
	Body           string  `json:"body"`
	Kind           string  `json:"kind"`
}

type PostMessageResponse struct {
	MessageID string `json:"message_id"`
	CreatedAt string `json:"created_at"`
}

type ListMessagesResponse struct {
	Messages   any    `json:"messages"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type ApprovalRequestCreate struct {
	ConversationID string          `json:"conversation_id"`
	AgentID        string          `json:"agent_id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Schema         json.RawMessage `json:"schema"`
	RiskLevel      string          `json:"risk_level"`
	ExpiresAt      string          `json:"expires_at"`
}

type ApprovalRequestCreateResponse struct {
	ApprovalID string `json:"approval_id"`
	Status     string `json:"status"`
}

type ApprovalRespondRequest struct {
	HumanID  string          `json:"human_id"`
	Decision string          `json:"decision"`
	Payload  json.RawMessage `json:"payload"`
}

type ApprovalRespondResponse struct {
	ApprovalID string `json:"approval_id"`
	Status     string `json:"status"`
	ResolvedAt string `json:"resolved_at"`
}

type DispatchRequest struct {
	ConversationID string         `json:"conversation_id"`
	Prompt         string         `json:"prompt"`
	Metadata       map[string]any `json:"metadata"`
}

type DispatchResponse struct {
	DispatchID string `json:"dispatch_id"`
	Status     string `json:"status"`
}
