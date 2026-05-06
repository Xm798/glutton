package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAllEventTypeConstantsExposed parses internal/events/bus.go and asserts
// every Type* string constant is present in allKnownEventTypes. This guards
// against the recurring drift bug where a new Type* is added but nobody
// remembers to expose it via /api/events/types — the FE settings picker
// would then silently drop it.
func TestAllEventTypeConstantsExposed(t *testing.T) {
	path := filepath.Join("..", "events", "bus.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err, "parse %s", path)

	declared := map[string]string{} // const name → string literal value
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Type") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				declared[name.Name] = val
			}
		}
	}
	require.NotEmpty(t, declared, "no Type* constants discovered in events/bus.go — did the file move?")

	exposed := make(map[string]struct{}, len(allKnownEventTypes))
	for _, v := range allKnownEventTypes {
		exposed[v] = struct{}{}
	}

	var missing []string
	for name, val := range declared {
		if _, ok := exposed[val]; !ok {
			missing = append(missing, name+" ("+val+")")
		}
	}
	require.Empty(t, missing,
		"event type constants declared in events/bus.go but not exposed via "+
			"allKnownEventTypes (api/event_types.go): %v", missing)
}
