package events

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcaster_SubscribeAndEmit(t *testing.T) {
	b := NewBroadcaster()

	var received []string
	unsub := b.Subscribe("session-1", func(event string, data interface{}) {
		received = append(received, event)
	})
	defer unsub()

	b.Emit("session-1", "code_update", map[string]string{"code": "print('hello')"})
	b.Emit("session-1", "cursor_move", nil)

	assert.Equal(t, []string{"code_update", "cursor_move"}, received)
}

func TestBroadcaster_EmitToWrongSession(t *testing.T) {
	b := NewBroadcaster()

	called := false
	unsub := b.Subscribe("session-1", func(event string, data interface{}) {
		called = true
	})
	defer unsub()

	b.Emit("session-2", "code_update", nil)
	assert.False(t, called, "should not receive events for other sessions")
}

func TestBroadcaster_Unsubscribe(t *testing.T) {
	b := NewBroadcaster()

	callCount := 0
	unsub := b.Subscribe("session-1", func(event string, data interface{}) {
		callCount++
	})

	b.Emit("session-1", "event1", nil)
	assert.Equal(t, 1, callCount)

	unsub()

	b.Emit("session-1", "event2", nil)
	assert.Equal(t, 1, callCount, "should not receive events after unsubscribe")
}

func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	b := NewBroadcaster()

	var count1, count2 int
	unsub1 := b.Subscribe("session-1", func(event string, data interface{}) { count1++ })
	unsub2 := b.Subscribe("session-1", func(event string, data interface{}) { count2++ })
	defer unsub1()
	defer unsub2()

	b.Emit("session-1", "test", nil)
	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}

func TestBroadcaster_UnsubscribeCleansUpSession(t *testing.T) {
	b := NewBroadcaster()

	unsub := b.Subscribe("session-1", func(event string, data interface{}) {})
	unsub()

	b.mu.RLock()
	_, exists := b.listeners["session-1"]
	b.mu.RUnlock()
	assert.False(t, exists, "session should be cleaned up after last subscriber unsubscribes")
}

func TestBroadcaster_ConcurrentAccess(t *testing.T) {
	b := NewBroadcaster()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := b.Subscribe("session-1", func(event string, data interface{}) {})
			b.Emit("session-1", "test", nil)
			unsub()
		}()
	}
	wg.Wait()
}

func TestBroadcaster_EmitData(t *testing.T) {
	b := NewBroadcaster()

	var receivedData interface{}
	unsub := b.Subscribe("session-1", func(event string, data interface{}) {
		receivedData = data
	})
	defer unsub()

	payload := map[string]string{"code": "x = 1"}
	b.Emit("session-1", "code_update", payload)

	require.NotNil(t, receivedData)
	assert.Equal(t, payload, receivedData)
}

func TestWriteSSE(t *testing.T) {
	var buf bytes.Buffer
	WriteSSE(&buf, "code_update", `{"code":"hello"}`)
	assert.Equal(t, "event: code_update\ndata: {\"code\":\"hello\"}\n\n", buf.String())
}

func TestWriteSSE_EmptyData(t *testing.T) {
	var buf bytes.Buffer
	WriteSSE(&buf, "ping", "")
	assert.Equal(t, "event: ping\ndata: \n\n", buf.String())
}
