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
	seen := make(map[string]bool)
	for _, b := range bs {
		require.NotEmpty(t, b.Name)
		require.NotEmpty(t, b.URLs, "builtin %q has no URLs", b.Name)
		for _, u := range b.URLs {
			require.NoError(t, sources.ValidateURL(u))
			require.False(t, seen[u], "duplicate URL: %s", u)
			seen[u] = true
		}
	}
}

func TestLoadBuiltinsHuaweiIsOneSource(t *testing.T) {
	bs, err := sources.LoadBuiltins()
	require.NoError(t, err)
	var huawei *sources.Builtin
	for i := range bs {
		if bs[i].Name == "Huawei video" {
			huawei = &bs[i]
			break
		}
	}
	require.NotNil(t, huawei, "Huawei video should be a single builtin")
	require.Len(t, huawei.URLs, 36, "Huawei video should carry all 36 URLs in one source")
}
