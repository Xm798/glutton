package sources

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

//go:embed builtin.json
var builtinJSON []byte

type Builtin struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	UA     string `json:"ua,omitempty"`
	Weight int    `json:"weight"`
}

func LoadBuiltins() ([]Builtin, error) {
	var out []Builtin
	if err := json.Unmarshal(builtinJSON, &out); err != nil {
		return nil, fmt.Errorf("decode builtin.json: %w", err)
	}
	for i := range out {
		if out[i].Weight <= 0 {
			out[i].Weight = 1
		}
	}
	return out, nil
}

type Candidate struct {
	ID            int64
	Name          string
	URL           string
	UA            string
	Weight        int
	CooldownUntil time.Time
}

type Pool struct {
	mu   sync.Mutex
	cand []Candidate
	rng  *rand.Rand
}

func NewPool(cs []Candidate, rng *rand.Rand) *Pool {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Pool{cand: append([]Candidate(nil), cs...), rng: rng}
}

// Replace swaps the candidate set atomically. Used when sources change in DB.
func (p *Pool) Replace(cs []Candidate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cand = append([]Candidate(nil), cs...)
}

// Pick returns a weighted-random eligible candidate, avoiding lastID when alternatives exist.
// lastID = -1 means "no previous pick". Returns ok=false if no eligible candidate.
func (p *Pool) Pick(now time.Time, lastID int64) (Candidate, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	eligible := p.cand[:0:0]
	for _, c := range p.cand {
		if c.Weight <= 0 {
			continue
		}
		if !c.CooldownUntil.IsZero() && c.CooldownUntil.After(now) {
			continue
		}
		eligible = append(eligible, c)
	}
	if len(eligible) == 0 {
		return Candidate{}, false
	}
	if len(eligible) > 1 {
		// Drop lastID if there's a real alternative.
		filtered := eligible[:0:0]
		for _, c := range eligible {
			if c.ID != lastID {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			eligible = filtered
		}
	}
	return weightedPick(eligible, p.rng), true
}

func weightedPick(cs []Candidate, rng *rand.Rand) Candidate {
	total := 0
	for _, c := range cs {
		total += c.Weight
	}
	r := rng.Intn(total)
	for _, c := range cs {
		if r < c.Weight {
			return c
		}
		r -= c.Weight
	}
	return cs[len(cs)-1] // unreachable, but satisfies the compiler
}
