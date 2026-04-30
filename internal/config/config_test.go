package config_test

import (
	"testing"

	"github.com/cyrus/glutton/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TZ", "")
	t.Setenv("GLUTTON_DATA_DIR", "")
	t.Setenv("GLUTTON_LISTEN", "")
	t.Setenv("GLUTTON_LOG_LEVEL", "")

	c, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "./data", c.DataDir)
	require.Equal(t, ":7890", c.Listen)
	require.Equal(t, "info", c.LogLevel)
	require.Equal(t, "Asia/Shanghai", c.TZ)
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("TZ", "UTC")
	t.Setenv("GLUTTON_DATA_DIR", "/tmp/g")
	t.Setenv("GLUTTON_LISTEN", "127.0.0.1:9999")
	t.Setenv("GLUTTON_LOG_LEVEL", "debug")

	c, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "/tmp/g", c.DataDir)
	require.Equal(t, "127.0.0.1:9999", c.Listen)
	require.Equal(t, "debug", c.LogLevel)
	require.Equal(t, "UTC", c.TZ)
}

func TestLoadRejectsBadLogLevel(t *testing.T) {
	t.Setenv("GLUTTON_LOG_LEVEL", "bogus")
	_, err := config.Load()
	require.Error(t, err)
}
