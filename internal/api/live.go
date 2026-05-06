package api

import (
	"sync"
	"time"
)

// LiveCounters holds in-memory aggregates pushed from the consumer.
// Read by /api/stats/live.
type LiveCounters struct {
	mu             sync.Mutex
	currentRateBps int64
	todayBytes     int64
	monthBytes     int64
	updated        time.Time
}

// Set stamps the current snapshot. updated_at advances on every call so the
// freshness indicator stays meaningful even when the rate is identically zero.
func (l *LiveCounters) Set(rateBps, today, month int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.currentRateBps = rateBps
	l.todayBytes = today
	l.monthBytes = month
	l.updated = time.Now()
}

func (l *LiveCounters) Snapshot() (rate, today, month int64, updated time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.currentRateBps, l.todayBytes, l.monthBytes, l.updated
}
