package sources

import (
	_ "embed"
	"encoding/json"
	"fmt"
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
