package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// helloHandler returns 200 to confirm the middleware permitted the request.
func helloHandler(c echo.Context) error { return c.String(http.StatusOK, "ok") }

func newAuthTestServer(t *testing.T, token string) http.Handler {
	t.Helper()
	t.Setenv(authEnvVar, token)
	resetAuthWarnOnceForTests()

	e := echo.New()
	e.Use(AuthMiddleware())
	e.GET("/api/whatever", helloHandler)
	e.GET("/api/version", helloHandler)
	e.GET("/metrics", helloHandler)
	e.GET("/", helloHandler) // SPA-ish non-/api
	return e
}

func TestM2AuthOpenWhenTokenUnset(t *testing.T) {
	srv := newAuthTestServer(t, "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/whatever", nil))
	require.Equal(t, http.StatusOK, rec.Code, "open mode: requests without a token are allowed")
}

func TestM2AuthRequiredWhenTokenSet(t *testing.T) {
	srv := newAuthTestServer(t, "secret-1234")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/whatever", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer secret-1234")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Basic abc")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code, "non-bearer schemes rejected")
}

func TestM2AuthAlwaysAllowedPaths(t *testing.T) {
	srv := newAuthTestServer(t, "secret")

	for _, p := range []string{"/metrics", "/api/version", "/"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		require.Equal(t, http.StatusOK, rec.Code, "path %q must bypass auth", p)
	}
}

// TestOpenModeWarnIsPeriodic verifies the no-auth reminder fires once on the
// first request and then re-fires on the configured interval, so a long-lived
// open-mode deployment keeps reminding operators in `tail -f` even after the
// startup line scrolls off.
func TestOpenModeWarnIsPeriodic(t *testing.T) {
	prev := noAuthWarnInterval
	noAuthWarnInterval = 20 * time.Millisecond
	t.Cleanup(func() { noAuthWarnInterval = prev })

	srv := newAuthTestServer(t, "")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/whatever", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	// One immediate emit + at least two ticker emits within ~80ms.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if noAuthWarnCount.Load() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.GreaterOrEqual(t, noAuthWarnCount.Load(), uint32(3),
		"expected immediate warn + at least two ticker re-emits, got %d", noAuthWarnCount.Load())
}

func TestM2AuthConstantTimeCompare(t *testing.T) {
	// Tokens of different lengths must still take the same time path
	// (constant-time compare). We can't time-assert reliably in CI, but at
	// least confirm the rejection works regardless of length skew.
	srv := newAuthTestServer(t, "abcdef")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer a")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer abcdefghi")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
