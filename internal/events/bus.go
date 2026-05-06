package events

import (
	"sync"
	"sync/atomic"
	"time"
)

// Event types emitted by the system. Centralised so consumers (UI, notifier)
// can match on stable strings.
const (
	TypeServiceStarted     = "service_started"
	TypeServiceStopped     = "service_stopped"
	TypeServicePaused      = "service_paused"
	TypeServiceResumed     = "service_resumed"
	TypeQuotaReachedDaily  = "quota_reached_daily"
	TypeQuotaReachedMonth  = "quota_reached_monthly"
	TypeDailyReset         = "daily_reset"
	TypeDailyResetManual   = "daily_reset_manual"
	TypeMonthlyReset       = "monthly_reset"
	TypeSourceError        = "source_error"
	TypeSourcesMassFailure = "sources_mass_failure"
	TypeBurstStarted       = "burst_started"
	TypeBurstEnded         = "burst_ended"
	TypeConfigUpdated      = "config_updated"
	TypeSourceCreated      = "source_created"
	TypeSourceUpdated      = "source_updated"
	TypeSourceDeleted      = "source_deleted"
	TypeStateChanged       = "state_changed"
	TypeSourceCooldown     = "source_cooldown"
)

// Event is the unit of publication on the bus. ID is a process-monotonic
// counter assigned by Publish; persistence layers store it as the canonical
// cross-channel id so SSE and /api/events/history can be deduped on the
// client by `id` alone.
type Event struct {
	ID      uint64    `json:"id"`
	TS      time.Time `json:"TS"`
	Type    string    `json:"Type"`
	Level   string    `json:"Level"`
	Message string    `json:"Message"`
	// Data is an optional payload for SSE consumers; not persisted.
	Data map[string]any `json:"Data,omitempty"`
}

type Bus struct {
	mu      sync.RWMutex
	subs    []chan Event
	bufSize int
	closed  bool
	nextID  atomic.Uint64
}

func NewBus(perSubBuffer int) *Bus {
	if perSubBuffer <= 0 {
		perSubBuffer = 16
	}
	return &Bus{bufSize: perSubBuffer}
}

// SetNextID seeds the monotonic id counter so the next Publish will assign
// `start + 1`. Intended to be called once at process startup with
// `store.MaxEventID(db)` to avoid id collisions with persisted history
// after a restart. Safe to call before any subscribers exist; not safe to
// race with Publish.
func (b *Bus) SetNextID(start uint64) {
	b.nextID.Store(start)
}

func (b *Bus) Subscribe() <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, b.bufSize)
	b.subs = append(b.subs, ch)
	return ch
}

// SubscribeBuffered is like Subscribe but allows the caller to choose the
// per-subscriber channel buffer. Useful for slow consumers (e.g. config
// debounce coalescer) that want a tighter buffer to coalesce drop-on-full.
func (b *Bus) SubscribeBuffered(buf int) <-chan Event {
	if buf <= 0 {
		buf = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, buf)
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
//
// Each Publish stamps a monotonically-increasing ID so persisted rows and
// live SSE frames share a key space; ID is reflected back via the same
// Event value, allowing the persist hook to write it to the DB.
func (b *Bus) Publish(e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	if e.ID == 0 {
		e.ID = b.nextID.Add(1)
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

// DefaultSubscribedTypes is the canonical list of event types operators are
// notified about by default on a fresh install. Extending this list is a
// product decision; loadRuntime in cmd/glutton seeds it when the
// `subscribed_events` config row is absent.
func DefaultSubscribedTypes() []string {
	return []string{
		TypeQuotaReachedDaily,
		TypeQuotaReachedMonth,
		TypeSourcesMassFailure,
		TypeSourceCooldown,
	}
}
