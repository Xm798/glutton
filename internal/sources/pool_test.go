package sources_test

import (
	"strings"
	"testing"

	"github.com/cyrus/glutton/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestLoadBuiltins(t *testing.T) {
	bs, err := sources.LoadBuiltins()
	require.NoError(t, err)
	require.NotEmpty(t, bs)
	seen := make(map[string]bool, len(bs))
	for _, b := range bs {
		require.NotEmpty(t, b.Name)
		require.NoError(t, sources.ValidateURL(b.URL))
		require.False(t, seen[b.URL], "duplicate URL: %s", b.URL)
		seen[b.URL] = true
	}
}

func TestLoadBuiltinsExpandsGroups(t *testing.T) {
	bs, err := sources.LoadBuiltins()
	require.NoError(t, err)
	count := 0
	for _, b := range bs {
		if strings.HasPrefix(b.Name, "Huawei video #") {
			count++
		}
	}
	require.Equal(t, 36, count, "Huawei video group should expand to 36 entries")
}
