package scheduler

import (
	"sync"
	"time"
)

type Status int

const (
	Idle Status = iota
	Running
	Paused
	QuotaReached
)

func (s Status) String() string {
	switch s {
	case Idle:
		return "idle"
	case Running:
		return "running"
	case Paused:
		return "paused"
	case QuotaReached:
		return "quota_reached"
	default:
		return "unknown"
	}
}

// TransitionHook fires whenever Get() would return a different value than
// before. Called synchronously while State's mutex is held; implementations
// must not call back into State.
type TransitionHook func(from, to Status)

type State struct {
	mu          sync.Mutex
	cur         Status
	paused      bool      // sticky: persists across Activate/Deactivate
	wasActive   bool      // latched by Activate, cleared by Deactivate; lets Resume restore Running
	burstUntil  time.Time // non-zero ⇒ a manual burst is in progress until this moment
	onTransition TransitionHook
	now         func() time.Time
}

func NewState() *State { return &State{cur: Idle, now: time.Now} }

// SetTransitionHook installs an observer for state changes. Replaces any
// previous hook. Pass nil to clear.
func (s *State) SetTransitionHook(h TransitionHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTransition = h
}

// SetNow overrides the clock used for burst expiry. Tests only.
func (s *State) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now == nil {
		s.now = time.Now
	} else {
		s.now = now
	}
}

func (s *State) Get() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// BurstActive reports whether a manual burst override is in effect (deadline
// in the future). Once the deadline has passed it is treated as inactive but
// the field is left as-is until EndBurst clears it.
func (s *State) BurstActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.burstActiveLocked()
}

func (s *State) burstActiveLocked() bool {
	if s.burstUntil.IsZero() {
		return false
	}
	return s.now().Before(s.burstUntil)
}

// Activate marks the scheduler as in-window. If paused, the observable state
// stays Paused but wasActive is latched so Resume can restore Running.
// If quota-reached, stays QuotaReached.
//
// burstUntil, when non-zero, marks this Activate as part of a manual burst
// override; subsequent Deactivate calls become no-ops until the deadline
// expires or EndBurst is called.
func (s *State) Activate(burstUntil ...time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(burstUntil) > 0 && !burstUntil[0].IsZero() {
		// Extend, never shrink: a fresh Burst replaces the deadline outright,
		// but Activate from the scheduler ticker should not clobber a longer
		// burst. Callers that intend to replace use EndBurst first.
		if burstUntil[0].After(s.burstUntil) {
			s.burstUntil = burstUntil[0]
		}
	}
	from := s.cur
	s.wasActive = true
	if s.paused {
		s.cur = Paused
	} else if s.cur != QuotaReached {
		s.cur = Running
	}
	s.notifyLocked(from)
	return nil
}

// Deactivate marks the scheduler as out-of-window. If currently Running, drops
// to Idle. Quota-reached and Paused states are not disturbed. Skipped while a
// manual burst is active.
func (s *State) Deactivate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.burstActiveLocked() {
		// Burst override holds the scheduler Running irrespective of cron.
		return nil
	}
	from := s.cur
	s.wasActive = false
	if s.cur == Running {
		s.cur = Idle
	}
	s.notifyLocked(from)
	return nil
}

// EndBurst clears any active burst deadline so the next Tick can Deactivate
// normally if the cron window is closed.
func (s *State) EndBurst() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.burstUntil = time.Time{}
}

func (s *State) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.cur
	s.paused = true
	s.cur = Paused
	s.notifyLocked(from)
	return nil
}

// Resume clears the paused flag and restores Running if Activate was called
// while paused (wasActive latched), else Idle.
func (s *State) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.cur
	s.paused = false
	if s.cur == Paused {
		if s.wasActive {
			s.cur = Running
		} else {
			s.cur = Idle
		}
	}
	s.notifyLocked(from)
	return nil
}

func (s *State) QuotaReached() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.cur
	s.cur = QuotaReached
	s.notifyLocked(from)
	return nil
}

func (s *State) ResetQuota() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.cur
	if s.cur == QuotaReached {
		s.cur = Idle
	}
	s.notifyLocked(from)
	return nil
}

func (s *State) notifyLocked(from Status) {
	if s.onTransition == nil || from == s.cur {
		return
	}
	s.onTransition(from, s.cur)
}
