package events

import (
	"fmt"
	"sync"
)

type EventCallback func(event string, data interface{})

type Broadcaster struct {
	mu        sync.RWMutex
	listeners map[string]map[int]EventCallback
	nextID    int
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		listeners: make(map[string]map[int]EventCallback),
	}
}

func (b *Broadcaster) Subscribe(sessionID string, cb EventCallback) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.listeners[sessionID] == nil {
		b.listeners[sessionID] = make(map[int]EventCallback)
	}

	id := b.nextID
	b.nextID++
	b.listeners[sessionID][id] = cb

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.listeners[sessionID], id)
		if len(b.listeners[sessionID]) == 0 {
			delete(b.listeners, sessionID)
		}
	}
}

func (b *Broadcaster) Emit(sessionID string, event string, data interface{}) {
	// Copy callbacks under lock, then invoke outside the lock.
	// This prevents slow subscribers from blocking other Emit/Subscribe calls.
	b.mu.RLock()
	subs, ok := b.listeners[sessionID]
	if !ok || len(subs) == 0 {
		b.mu.RUnlock()
		return
	}
	cbs := make([]EventCallback, 0, len(subs))
	for _, cb := range subs {
		cbs = append(cbs, cb)
	}
	b.mu.RUnlock()

	for _, cb := range cbs {
		cb(event, data)
	}
}

// WriteSSE writes a Server-Sent Event to an http.ResponseWriter.
func WriteSSE(w interface{ Write([]byte) (int, error) }, event string, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
