package events

import (
	"sync"
	"time"
)

type Event struct {
	TS      time.Time
	Type    string // e.g., "quota_reached_daily", "sources_mass_failure"
	Level   string // "info" | "warn" | "error"
	Message string
	// Data is an optional payload for SSE consumers; not persisted.
	Data map[string]any
}

type Bus struct {
	mu      sync.RWMutex
	subs    []chan Event
	bufSize int
	closed  bool
}

func NewBus(perSubBuffer int) *Bus {
	if perSubBuffer <= 0 {
		perSubBuffer = 16
	}
	return &Bus{bufSize: perSubBuffer}
}

func (b *Bus) Subscribe() <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, b.bufSize)
	b.subs = append(b.subs, ch)
	return ch
}

func (b *Bus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, c := range b.subs {
		if (<-chan Event)(c) == ch {
			close(c)
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}

// Publish is non-blocking: if a subscriber's buffer is full, the event is
// dropped for that subscriber. Slow consumers must not stall producers.
func (b *Bus) Publish(e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}
