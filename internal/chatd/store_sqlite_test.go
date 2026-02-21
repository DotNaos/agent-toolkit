package chatd

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T, leaseDuration time.Duration) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "chat.db"), leaseDuration)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestSQLiteStore_FIFOLeaseOrder(t *testing.T) {
	store := newTestStore(t, DefaultLeaseDuration)

	m1, err := store.EnqueueMessage(EnqueueParams{ToAgent: "agent-a", Body: "first"})
	if err != nil {
		t.Fatalf("enqueue first failed: %v", err)
	}
	_, err = store.EnqueueMessage(EnqueueParams{ToAgent: "agent-a", Body: "second"})
	if err != nil {
		t.Fatalf("enqueue second failed: %v", err)
	}

	lease1, err := store.LeaseNextMessage(LeaseParams{Agent: "agent-a"})
	if err != nil {
		t.Fatalf("first lease failed: %v", err)
	}
	if lease1 == nil {
		t.Fatal("expected first lease, got nil")
	}
	if lease1.ID != m1.ID {
		t.Fatalf("expected first leased message %s, got %s", m1.ID, lease1.ID)
	}

	if _, err := store.AckMessage(AckParams{Agent: "agent-a", MessageID: lease1.ID, LeaseToken: lease1.LeaseToken}); err != nil {
		t.Fatalf("ack first failed: %v", err)
	}

	lease2, err := store.LeaseNextMessage(LeaseParams{Agent: "agent-a"})
	if err != nil {
		t.Fatalf("second lease failed: %v", err)
	}
	if lease2 == nil {
		t.Fatal("expected second lease, got nil")
	}
	if lease2.Body != "second" {
		t.Fatalf("expected second message body, got %q", lease2.Body)
	}
}

func TestSQLiteStore_AckRejectsInvalidLease(t *testing.T) {
	store := newTestStore(t, DefaultLeaseDuration)

	msg, err := store.EnqueueMessage(EnqueueParams{ToAgent: "agent-a", Body: "payload"})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	lease, err := store.LeaseNextMessage(LeaseParams{Agent: "agent-a"})
	if err != nil {
		t.Fatalf("lease failed: %v", err)
	}
	if lease == nil {
		t.Fatal("expected lease, got nil")
	}

	_, err = store.AckMessage(AckParams{Agent: "agent-a", MessageID: msg.ID, LeaseToken: "wrong-token"})
	if !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("expected ErrLeaseConflict, got %v", err)
	}
}

func TestSQLiteStore_RedeliversExpiredLease(t *testing.T) {
	store := newTestStore(t, 150*time.Millisecond)

	msg, err := store.EnqueueMessage(EnqueueParams{ToAgent: "agent-a", Body: "payload"})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	lease1, err := store.LeaseNextMessage(LeaseParams{Agent: "agent-a"})
	if err != nil {
		t.Fatalf("first lease failed: %v", err)
	}
	if lease1 == nil {
		t.Fatal("expected first lease, got nil")
	}

	time.Sleep(200 * time.Millisecond)

	lease2, err := store.LeaseNextMessage(LeaseParams{Agent: "agent-a"})
	if err != nil {
		t.Fatalf("second lease failed: %v", err)
	}
	if lease2 == nil {
		t.Fatal("expected second lease, got nil")
	}
	if lease2.ID != msg.ID {
		t.Fatalf("expected re-leased message %s, got %s", msg.ID, lease2.ID)
	}
	if lease2.LeaseToken == lease1.LeaseToken {
		t.Fatal("expected lease token to change on redelivery")
	}
	if lease2.Attempt < 2 {
		t.Fatalf("expected attempt_count >=2, got %d", lease2.Attempt)
	}
}
