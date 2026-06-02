package sources

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// validateDNSTimeoutEnvVar lets operators tune the DNS lookup deadline used
// during URL validation. Defaults to 3s — long enough for slow public DNS,
// short enough to keep CRUD responsive. Internal-only deployments can drop
// it; operators with very-slow international DNS can bump it.
const validateDNSTimeoutEnvVar = "GLUTTON_VALIDATE_DNS_TIMEOUT_MS"

const defaultValidateDNSTimeout = 3 * time.Second

func validateDNSTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv(validateDNSTimeoutEnvVar)); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultValidateDNSTimeout
}

// ValidateURL screens a candidate source URL against an SSRF blacklist.
//
// In addition to the v1 literal-IP guard (loopback / RFC1918 / link-local /
// unspecified) it now also resolves DNS hostnames at validation time and
// rejects the URL if ANY resolution lands on a private range. This still
// can't defend against DNS rebinding at request time — the per-request
// DialContext below covers that.
func ValidateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("url is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host")
	}

	// Literal IP path.
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("host %s is loopback/private/link-local/metadata", host)
		}
		return nil
	}

	// Hostname path: also reject the literal "localhost" without paying for DNS.
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("host %q resolves to loopback", host)
	}

	// Resolve and screen every answer. A single private resolution disqualifies
	// the URL — refusing is safer than picking only the public answer because
	// resolution order is non-deterministic.
	ctx, cancel := context.WithTimeout(context.Background(), validateDNSTimeout())
	defer cancel()
	ips, err := defaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		// NXDOMAIN-style failures: reject. Don't leak resolver errors back to
		// the operator beyond a short reason; callers log on the boundary.
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("host %s resolves to blocked address %s", host, ip)
		}
	}
	return nil
}

// ValidateURLs screens every URL and requires at least one. Each distinct host
// is DNS-screened once — a URL group often shares a host (e.g. 36 huawei URLs),
// and ValidateURL does a live DNS lookup per call, so deduping by host avoids N
// redundant lookups. URLs reusing an already-screened host still get a cheap
// scheme/parse check.
func ValidateURLs(urls []string) error {
	if len(urls) == 0 {
		return fmt.Errorf("at least one url is required")
	}
	screened := make(map[string]bool, len(urls))
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("parse url %q: %w", raw, err)
		}
		if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
			return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
		}
		host := u.Hostname()
		if screened[host] {
			continue
		}
		if err := ValidateURL(raw); err != nil {
			return err
		}
		screened[host] = true
	}
	return nil
}

// defaultResolver is overridable in tests so we can simulate CNAMEs into
// private space without touching real DNS.
var defaultResolver Resolver = net.DefaultResolver

// Resolver is the minimal subset of net.Resolver used by ValidateURL.
type Resolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// SetResolver swaps the package-level resolver. Returns the previous
// resolver so tests can defer-restore.
func SetResolver(r Resolver) Resolver {
	prev := defaultResolver
	defaultResolver = r
	return prev
}

// metadataIPs covers the well-known cloud-instance-metadata addresses we
// want blocked even though they're technically link-local and would be
// caught by IsLinkLocalUnicast — listing them explicitly makes the intent
// readable and gives us coverage in case Go's classification ever shifts.
var metadataIPs = []string{
	"169.254.169.254", // AWS / GCP / DigitalOcean / Oracle
	"100.100.100.200", // Alibaba Cloud
	"fd00:ec2::254",   // AWS IPv6 IMDS
}

// isBlockedIP returns true if ip is in any range an SSRF screen should refuse.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, m := range metadataIPs {
		if ip.Equal(net.ParseIP(m)) {
			return true
		}
	}
	// Carrier-grade NAT (100.64.0.0/10) — IsPrivate doesn't include it.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	// IPv4-mapped IPv6: re-check as IPv4.
	if v4 := ip.To4(); v4 != nil && len(ip) == 16 {
		return isBlockedIP(v4)
	}
	return false
}

// SafeDialContextFunc returns a net.Dialer Control that aborts the connect
// at the syscall layer if the resolved peer address falls inside the SSRF
// blacklist. This closes the DNS-rebinding window: even if a hostname's A
// record flips between Validate-time and request-time, the actual TCP peer
// is still vetted before SYN is sent.
//
// Use it like:
//
//	dialer := &net.Dialer{Control: sources.SafeDialerControl}
//	transport := &http.Transport{DialContext: dialer.DialContext}
func SafeDialerControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Defensive: shouldn't happen; net.Dialer hands a literal IP here.
		return errors.New("dial address has no IP")
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("dial blocked: %s in private/loopback/metadata range", ip)
	}
	return nil
}
