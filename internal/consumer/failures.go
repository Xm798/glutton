package consumer

import (
	"sync"
	"time"
)

// FailureTracker keeps a rolling window of attempt outcomes for mass-failure
// detection. Thread-safe.
type FailureTracker struct {
	mu       sync.Mutex
	window   time.Duration
	attempts []attempt
	now      func() time.Time
}

type attempt struct {
	ts       time.Time
	failed   bool
	sourceID uint
}

func NewFailureTracker(window time.Duration) *FailureTracker {
	return &FailureTracker{window: window, now: time.Now}
}

// Record adds an attempt outcome to the rolling window.
func (f *FailureTracker) Record(sourceID uint, failed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gcLocked()
	f.attempts = append(f.attempts, attempt{ts: f.now(), failed: failed, sourceID: sourceID})
}

// FailureRatio returns (failureCount, distinctSources, ratio) over the window.
// ratio is 0..1; distinctSources is the count of unique sourceIDs seen.
func (f *FailureTracker) FailureRatio() (failed, distinct int, ratio float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gcLocked()
	if len(f.attempts) == 0 {
		return 0, 0, 0
	}
	seen := make(map[uint]struct{}, len(f.attempts))
	for _, a := range f.attempts {
		seen[a.sourceID] = struct{}{}
		if a.failed {
			failed++
		}
	}
	distinct = len(seen)
	ratio = float64(failed) / float64(len(f.attempts))
	return
}

// AttemptCount returns total attempts in the window (for the min-attempts gate).
func (f *FailureTracker) AttemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gcLocked()
	return len(f.attempts)
}

func (f *FailureTracker) gcLocked() {
	cutoff := f.now().Add(-f.window)
	keep := f.attempts[:0]
	for _, a := range f.attempts {
		if a.ts.After(cutoff) {
			keep = append(keep, a)
		}
	}
	f.attempts = keep
}
