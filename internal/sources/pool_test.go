package sources_test

import (
	"testing"

	"github.com/cyrus/glutton/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestLoadBuiltins(t *testing.T) {
	bs, err := sources.LoadBuiltins()
	require.NoError(t, err)
	require.NotEmpty(t, bs)
	for _, b := range bs {
		require.NotEmpty(t, b.Name)
		require.NoError(t, sources.ValidateURL(b.URL))
	}
}
