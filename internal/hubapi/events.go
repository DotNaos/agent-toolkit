package hubapi

import (
	"sync"

	"agent-toolkit/internal/hubworker"
)

type subscriber struct {
	ch             chan hubworker.Event
	conversationID string
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[int]subscriber
	nextID      int
}

func NewBroker() *Broker {
	return &Broker{subscribers: make(map[int]subscriber)}
}

func (b *Broker) Publish(event hubworker.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subscribers {
		if sub.conversationID != "" && sub.conversationID != event.ConversationID {
			continue
		}
		select {
		case sub.ch <- event:
		default:
		}
	}
}

func (b *Broker) Subscribe(conversationID string) (int, <-chan hubworker.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan hubworker.Event, 64)
	b.subscribers[id] = subscriber{ch: ch, conversationID: conversationID}
	return id, ch
}

func (b *Broker) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub, ok := b.subscribers[id]
	if !ok {
		return
	}
	delete(b.subscribers, id)
	close(sub.ch)
}
