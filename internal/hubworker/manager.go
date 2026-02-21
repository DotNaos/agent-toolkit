package hubworker

import (
	"encoding/json"
	"fmt"
	"time"

	"agent-toolkit/internal/hubstore"
)

type Event struct {
	Type           string         `json:"type"`
	ConversationID string         `json:"conversation_id"`
	Data           map[string]any `json:"data"`
}

type EventPublisher interface {
	Publish(event Event)
}

type Manager struct {
	store           *hubstore.Store
	publisher       EventPublisher
	approvalTimeout time.Duration
	pollInterval    time.Duration
}

func NewManager(store *hubstore.Store, publisher EventPublisher) *Manager {
	return &Manager{
		store:           store,
		publisher:       publisher,
		approvalTimeout: 15 * time.Minute,
		pollInterval:    1 * time.Second,
	}
}

func (m *Manager) DispatchAgent(conversationID, agentID, prompt string, metadata map[string]any) (*hubstore.AgentDispatch, error) {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to encode metadata: %w", err)
	}

	dispatch, err := m.store.CreateDispatch(hubstore.CreateDispatchParams{
		ConversationID: conversationID,
		AgentID:        agentID,
		Prompt:         prompt,
		MetadataJSON:   string(metadataJSON),
		Status:         hubstore.DispatchStatusRunning,
	})
	if err != nil {
		return nil, err
	}

	m.publish("agent.status", conversationID, map[string]any{
		"dispatch_id": dispatch.ID,
		"agent_id":    agentID,
		"status":      string(hubstore.DispatchStatusRunning),
	})

	risky, reason := IsRiskyAction(prompt, metadata)
	if !risky {
		msg, err := m.store.AddMessage(hubstore.AddMessageParams{
			ConversationID: conversationID,
			FromID:         agentID,
			Kind:           hubstore.MessageKindSystem,
			Body:           fmt.Sprintf("Auto-approved (read/analyze). Reason: %s", reason),
		})
		if err == nil {
			m.publish("message.created", conversationID, map[string]any{"message": msg})
		}
		_ = m.store.UpdateDispatchStatus(dispatch.ID, hubstore.DispatchStatusCompleted)
		m.publish("agent.status", conversationID, map[string]any{
			"dispatch_id": dispatch.ID,
			"agent_id":    agentID,
			"status":      string(hubstore.DispatchStatusCompleted),
		})
		dispatch.Status = hubstore.DispatchStatusCompleted
		dispatch.UpdatedAt = hubstore.NowTimestamp()
		return dispatch, nil
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"decision": map[string]any{
				"type": "string",
				"enum": []string{"approve", "reject", "select"},
			},
			"options": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "enum": []string{"full", "limited", "dry-run"}},
			},
			"note": map[string]any{
				"type": "string",
			},
		},
	}
	schemaJSON, _ := json.Marshal(schema)

	approval, err := m.store.CreateApprovalRequest(hubstore.CreateApprovalRequestParams{
		ConversationID: conversationID,
		AgentID:        agentID,
		Title:          "Approval required for risky action",
		Description:    fmt.Sprintf("Dispatch blocked: %s", reason),
		SchemaJSON:     string(schemaJSON),
		RiskLevel:      "high",
		ExpiresAt:      time.Now().UTC().Add(m.approvalTimeout),
	})
	if err != nil {
		return nil, err
	}

	_ = m.store.UpdateDispatchStatus(dispatch.ID, hubstore.DispatchStatusWaitingApproval)
	m.publish("approval.requested", conversationID, map[string]any{"approval": approval, "dispatch_id": dispatch.ID})
	m.publish("agent.status", conversationID, map[string]any{
		"dispatch_id": dispatch.ID,
		"agent_id":    agentID,
		"status":      string(hubstore.DispatchStatusWaitingApproval),
	})

	go m.awaitApproval(dispatch.ID, approval.ID, conversationID, agentID, prompt)

	dispatch.Status = hubstore.DispatchStatusWaitingApproval
	return dispatch, nil
}

func (m *Manager) awaitApproval(dispatchID, approvalID, conversationID, agentID, prompt string) {
	for {
		_ = m.expireApprovals()
		approval, err := m.store.GetApprovalRequest(approvalID)
		if err != nil {
			_ = m.store.UpdateDispatchStatus(dispatchID, hubstore.DispatchStatusFailed)
			m.publish("agent.status", conversationID, map[string]any{
				"dispatch_id": dispatchID,
				"agent_id":    agentID,
				"status":      string(hubstore.DispatchStatusFailed),
				"reason":      err.Error(),
			})
			return
		}

		switch approval.Status {
		case hubstore.ApprovalStatusPending:
			time.Sleep(m.pollInterval)
			continue
		case hubstore.ApprovalStatusApproved, hubstore.ApprovalStatusSelected:
			_ = m.store.UpdateDispatchStatus(dispatchID, hubstore.DispatchStatusRunning)
			m.publish("agent.status", conversationID, map[string]any{
				"dispatch_id": dispatchID,
				"agent_id":    agentID,
				"status":      string(hubstore.DispatchStatusRunning),
			})
			msg, _ := m.store.AddMessage(hubstore.AddMessageParams{
				ConversationID: conversationID,
				FromID:         agentID,
				Kind:           hubstore.MessageKindSystem,
				Body:           fmt.Sprintf("Approval resolved (%s). Executed prompt: %s", approval.Status, prompt),
			})
			if msg != nil {
				m.publish("message.created", conversationID, map[string]any{"message": msg})
			}
			_ = m.store.UpdateDispatchStatus(dispatchID, hubstore.DispatchStatusCompleted)
			m.publish("agent.status", conversationID, map[string]any{
				"dispatch_id": dispatchID,
				"agent_id":    agentID,
				"status":      string(hubstore.DispatchStatusCompleted),
			})
			m.publish("approval.resolved", conversationID, map[string]any{"approval": approval, "dispatch_id": dispatchID})
			return
		case hubstore.ApprovalStatusRejected, hubstore.ApprovalStatusExpired:
			msg, _ := m.store.AddMessage(hubstore.AddMessageParams{
				ConversationID: conversationID,
				FromID:         agentID,
				Kind:           hubstore.MessageKindSystem,
				Body:           fmt.Sprintf("Action blocked: approval %s", approval.Status),
			})
			if msg != nil {
				m.publish("message.created", conversationID, map[string]any{"message": msg})
			}
			_ = m.store.UpdateDispatchStatus(dispatchID, hubstore.DispatchStatusRejected)
			m.publish("agent.status", conversationID, map[string]any{
				"dispatch_id": dispatchID,
				"agent_id":    agentID,
				"status":      string(hubstore.DispatchStatusRejected),
			})
			m.publish("approval.resolved", conversationID, map[string]any{"approval": approval, "dispatch_id": dispatchID})
			return
		default:
			time.Sleep(m.pollInterval)
		}
	}
}

func (m *Manager) expireApprovals() error {
	_, err := m.store.MarkExpiredApprovals(time.Now().UTC())
	return err
}

func (m *Manager) publish(eventType, conversationID string, data map[string]any) {
	if m.publisher == nil {
		return
	}
	m.publisher.Publish(Event{Type: eventType, ConversationID: conversationID, Data: data})
}
