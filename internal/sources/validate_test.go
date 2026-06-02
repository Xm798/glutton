package sources_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/sources"
	"github.com/stretchr/testify/require"
)

// blockingResolver hangs LookupIP until ctx expires; used to exercise the
// DNS-timeout knob in ValidateURL.
type blockingResolver struct{}

func (blockingResolver) LookupIP(ctx context.Context, _, _ string) ([]net.IP, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// stubResolver returns canned answers; tests use it to avoid real DNS.
type stubResolver map[string][]net.IP

func (s stubResolver) LookupIP(_ context.Context, _, host string) ([]net.IP, error) {
	if ips, ok := s[host]; ok {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func TestValidateURL(t *testing.T) {
	prev := sources.SetResolver(stubResolver{
		"speed.hetzner.de": {net.ParseIP("88.198.248.254")},
		"example.com":      {net.ParseIP("93.184.216.34")},
		"public.test":      {net.ParseIP("1.1.1.1")},
		"rebind.test":      {net.ParseIP("10.0.0.5")}, // CNAME-into-private trap
		"mixed.test":       {net.ParseIP("8.8.8.8"), net.ParseIP("192.168.0.5")},
	})
	t.Cleanup(func() { sources.SetResolver(prev) })

	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"https public", "https://speed.hetzner.de/100MB.bin", true},
		{"http public", "http://example.com/file", true},
		{"public test stub", "http://public.test/x", true},
		{"ftp scheme", "ftp://x/y", false},
		{"loopback", "http://127.0.0.1/x", false},
		{"loopback v6", "http://[::1]/x", false},
		{"rfc1918 10/8", "http://10.0.0.1/x", false},
		{"rfc1918 192.168", "http://192.168.1.1/x", false},
		{"rfc1918 172.16", "http://172.16.0.1/x", false},
		{"link-local", "http://169.254.1.1/x", false},
		{"aws metadata", "http://169.254.169.254/latest/meta-data/", false},
		{"alibaba metadata", "http://100.100.100.200/", false},
		{"cgnat", "http://100.64.0.1/", false},
		{"localhost word", "http://localhost/x", false},
		{"empty", "", false},
		{"junk", "://bogus", false},
		{"unknown host", "http://nx.invalid/x", false},
		// SSRF via DNS: the hostname looks innocuous but resolves into RFC1918.
		{"cname into private", "http://rebind.test/x", false},
		// One private IP among multiple resolutions still disqualifies.
		{"mixed answers public+private", "http://mixed.test/x", false},
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

// TestValidateDNSTimeoutEnvVar confirms GLUTTON_VALIDATE_DNS_TIMEOUT_MS
// shortens the DNS lookup deadline. Without the env var the default 3s
// would make this test take 3s; with 80ms it returns within ~150ms.
func TestValidateDNSTimeoutEnvVar(t *testing.T) {
	t.Setenv("GLUTTON_VALIDATE_DNS_TIMEOUT_MS", "80")
	prev := sources.SetResolver(blockingResolver{})
	t.Cleanup(func() { sources.SetResolver(prev) })

	start := time.Now()
	err := sources.ValidateURL("http://hangs.test/x")
	elapsed := time.Since(start)
	require.Error(t, err, "blocking resolver must produce a timeout error")
	require.Less(t, elapsed, 800*time.Millisecond,
		"validate should honour the env-var timeout, took %v", elapsed)
	require.Greater(t, elapsed, 50*time.Millisecond,
		"validate returned suspiciously fast (%v) — env var may not be honoured", elapsed)
}

func TestValidateURLs(t *testing.T) {
	require.Error(t, sources.ValidateURLs(nil), "empty list rejected")
	require.Error(t, sources.ValidateURLs([]string{"https://example.com/a", "ftp://example.com/b"}), "bad scheme rejected")
	require.Error(t, sources.ValidateURLs([]string{"http://10.0.0.1/x"}), "private IP rejected")
	require.NoError(t, sources.ValidateURLs([]string{"https://example.com/a", "https://example.com/b"}), "two same-host URLs accepted")
}

func TestSafeDialerControlRejectsPrivateIPs(t *testing.T) {
	cases := []struct {
		addr string
		ok   bool
	}{
		{"8.8.8.8:443", true},
		{"127.0.0.1:80", false},
		{"10.0.0.1:443", false},
		{"192.168.1.1:80", false},
		{"169.254.169.254:80", false},
		{"[::1]:443", false},
		{"100.64.0.1:443", false}, // CGNAT
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			err := sources.SafeDialerControl("tcp", tc.addr, nil)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
