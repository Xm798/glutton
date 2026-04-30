package sources_test

import (
	"testing"

	"github.com/cyrus/glutton/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestValidateURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"https public", "https://speed.hetzner.de/100MB.bin", true},
		{"http public", "http://example.com/file", true},
		{"ftp scheme", "ftp://x/y", false},
		{"loopback", "http://127.0.0.1/x", false},
		{"loopback v6", "http://[::1]/x", false},
		{"rfc1918 10/8", "http://10.0.0.1/x", false},
		{"rfc1918 192.168", "http://192.168.1.1/x", false},
		{"rfc1918 172.16", "http://172.16.0.1/x", false},
		{"link-local", "http://169.254.1.1/x", false},
		{"empty", "", false},
		{"junk", "://bogus", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := sources.ValidateURL(tc.url)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
