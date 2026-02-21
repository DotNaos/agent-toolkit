package hubstore

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func createTestConversation(t *testing.T, store *Store) *Conversation {
	t.Helper()
	conv, err := store.CreateConversation(CreateConversationParams{
		Name:         "Test",
		Participants: []ParticipantInput{{Type: ParticipantTypeHuman, ID: "owner"}, {Type: ParticipantTypeAgent, ID: "agent-a"}},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	return conv
}

func TestStoreApprovalLifecycle(t *testing.T) {
	store := newTestStore(t)
	conv := createTestConversation(t, store)

	approval, err := store.CreateApprovalRequest(CreateApprovalRequestParams{
		ConversationID: conv.ID,
		AgentID:        "agent-a",
		Title:          "Approve",
		Description:    "Need access",
		SchemaJSON:     `{"type":"object"}`,
		RiskLevel:      "high",
		ExpiresAt:      time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create approval failed: %v", err)
	}

	updated, err := store.RespondApproval(RespondApprovalParams{
		ApprovalID:  approval.ID,
		HumanID:     "owner",
		Decision:    "approve",
		PayloadJSON: `{"decision":"approve"}`,
	})
	if err != nil {
		t.Fatalf("respond approval failed: %v", err)
	}
	if updated.Status != ApprovalStatusApproved {
		t.Fatalf("expected approved status, got %s", updated.Status)
	}
	if updated.ResolvedAt == nil {
		t.Fatal("expected resolved_at to be set")
	}
}

func TestStoreApprovalRejectsExpiredResponse(t *testing.T) {
	store := newTestStore(t)
	conv := createTestConversation(t, store)

	approval, err := store.CreateApprovalRequest(CreateApprovalRequestParams{
		ConversationID: conv.ID,
		AgentID:        "agent-a",
		Title:          "Approve",
		Description:    "Need access",
		SchemaJSON:     `{"type":"object"}`,
		RiskLevel:      "high",
		ExpiresAt:      time.Now().UTC().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create approval failed: %v", err)
	}

	_, err = store.RespondApproval(RespondApprovalParams{
		ApprovalID:  approval.ID,
		HumanID:     "owner",
		Decision:    "approve",
		PayloadJSON: `{"decision":"approve"}`,
	})
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for expired approval, got %v", err)
	}

	reloaded, err := store.GetApprovalRequest(approval.ID)
	if err != nil {
		t.Fatalf("get approval failed: %v", err)
	}
	if reloaded.Status != ApprovalStatusExpired {
		t.Fatalf("expected status expired, got %s", reloaded.Status)
	}
}

func TestStoreMessagePaginationCursor(t *testing.T) {
	store := newTestStore(t)
	conv := createTestConversation(t, store)

	for i := 0; i < 3; i++ {
		if _, err := store.AddMessage(AddMessageParams{
			ConversationID: conv.ID,
			FromID:         "owner",
			Kind:           MessageKindText,
			Body:           "msg",
		}); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	firstPage, cursor, err := store.ListMessages(conv.ID, "", 2)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("expected 2 messages on first page, got %d", len(firstPage))
	}
	if cursor == "" {
		t.Fatal("expected non-empty cursor")
	}

	secondPage, nextCursor, err := store.ListMessages(conv.ID, cursor, 2)
	if err != nil {
		t.Fatalf("list messages second page failed: %v", err)
	}
	if len(secondPage) != 1 {
		t.Fatalf("expected 1 message on second page, got %d", len(secondPage))
	}
	if nextCursor != "" {
		t.Fatalf("expected empty next cursor, got %q", nextCursor)
	}
}
