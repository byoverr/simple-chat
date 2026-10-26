package httphandler

import (
	"sync"

	chatv1 "github.com/byoverr/auth-proto/gen/go/chat/v1"
)

// SSEBroker is an in-memory pub/sub for Server-Sent Events.
// When a message is sent to a chat, all SSE subscribers in that chat receive it.
type SSEBroker struct {
	mu   sync.RWMutex
	subs map[string]map[chan *chatv1.Message]struct{} // chatID -> set of subscriber channels
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		subs: make(map[string]map[chan *chatv1.Message]struct{}),
	}
}

func (b *SSEBroker) Subscribe(chatID string) chan *chatv1.Message {
	ch := make(chan *chatv1.Message, 64)
	b.mu.Lock()
	if b.subs[chatID] == nil {
		b.subs[chatID] = make(map[chan *chatv1.Message]struct{})
	}
	b.subs[chatID][ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *SSEBroker) Unsubscribe(chatID string, ch chan *chatv1.Message) {
	b.mu.Lock()
	if subs, ok := b.subs[chatID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(b.subs, chatID)
		}
	}
	b.mu.Unlock()
	close(ch)
}

// Publish implements usecase.Broker.
func (b *SSEBroker) Publish(chatID string, msg *chatv1.Message) {
	b.mu.RLock()
	subs := b.subs[chatID]
	b.mu.RUnlock()

	for ch := range subs {
		select {
		case ch <- msg:
		default:
			// drop if subscriber is slow
		}
	}
}
