package scheduler

import "sync"

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

type State struct {
	mu        sync.Mutex
	cur       Status
	paused    bool // sticky: persists across Activate/Deactivate
	wasActive bool // latched by Activate, cleared by Deactivate; lets Resume restore Running
}

func NewState() *State { return &State{cur: Idle} }

func (s *State) Get() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Activate marks the scheduler as in-window. If paused, the observable state
// stays Paused but wasActive is latched so Resume can restore Running.
// If quota-reached, stays QuotaReached.
func (s *State) Activate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wasActive = true
	if s.paused {
		s.cur = Paused
		return nil
	}
	if s.cur == QuotaReached {
		return nil
	}
	s.cur = Running
	return nil
}

// Deactivate marks the scheduler as out-of-window. If currently Running, drops
// to Idle. Quota-reached and Paused states are not disturbed.
func (s *State) Deactivate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wasActive = false
	if s.cur == Running {
		s.cur = Idle
	}
	return nil
}

func (s *State) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
	s.cur = Paused
	return nil
}

// Resume clears the paused flag and restores Running if Activate was called
// while paused (wasActive latched), else Idle.
func (s *State) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
	if s.cur != Paused {
		return nil
	}
	if s.wasActive {
		s.cur = Running
	} else {
		s.cur = Idle
	}
	return nil
}

func (s *State) QuotaReached() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = QuotaReached
	return nil
}

func (s *State) ResetQuota() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == QuotaReached {
		s.cur = Idle
	}
	return nil
}
