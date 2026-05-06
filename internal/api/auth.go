package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

const authEnvVar = "GLUTTON_AUTH_TOKEN"

// noAuthWarnInterval is how often the open-mode reminder repeats after the
// first request. Overridable in tests.
var noAuthWarnInterval = time.Hour

// noAuthWarnCount is incremented every time the open-mode reminder fires,
// for tests.
var noAuthWarnCount atomic.Uint32

var (
	noAuthWarnMu   sync.Mutex
	noAuthWarnOnce sync.Once
	noAuthWarnStop chan struct{}
)

func emitNoAuthWarn() {
	slog.Warn("running without auth: set " + authEnvVar +
		" to require a bearer token, or front the service " +
		"with a reverse-proxy that adds auth")
	noAuthWarnCount.Add(1)
}

// startNoAuthWarnLoop fires the reminder immediately, then re-emits it on a
// ticker so a long-lived deployment keeps reminding `tail -f` operators.
// The ticker goroutine listens on noAuthWarnStop so resetAuthWarnOnceForTests
// can shut it down between tests.
func startNoAuthWarnLoop() {
	emitNoAuthWarn()
	noAuthWarnMu.Lock()
	stop := make(chan struct{})
	noAuthWarnStop = stop
	interval := noAuthWarnInterval
	noAuthWarnMu.Unlock()

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				emitNoAuthWarn()
			case <-stop:
				return
			}
		}
	}()
}

// AuthMiddleware enforces an optional bearer-token gate.
//
// Behaviour:
//   - GLUTTON_AUTH_TOKEN unset (or empty): permit all, but log a single
//     WARN on first request so operators get a one-shot reminder that the
//     instance is open. /metrics, /api/version and the SPA static assets
//     are always allowed.
//   - GLUTTON_AUTH_TOKEN set: require `Authorization: Bearer <token>` on
//     all /api/* routes; constant-time compare; missing/wrong token → 401.
//
// Static SPA paths (non-/api, non-/metrics) bypass auth so unauthenticated
// users still see the login-prompt UI; the UI then attaches the token to
// API calls. Token storage in the SPA is the caller's responsibility — for
// most deployments a reverse-proxy doing TLS + auth is preferred.
func AuthMiddleware() echo.MiddlewareFunc {
	want := strings.TrimSpace(os.Getenv(authEnvVar))
	openMode := want == ""
	wantBytes := []byte(want)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			// Always-public paths (no auth, no warning).
			switch {
			case path == "/metrics",
				path == "/api/version":
				return next(c)
			case !strings.HasPrefix(path, "/api"):
				// SPA static / catch-all.
				return next(c)
			}

			if openMode {
				noAuthWarnOnce.Do(startNoAuthWarnLoop)
				return next(c)
			}

			h := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}
			got := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			if subtle.ConstantTimeCompare([]byte(got), wantBytes) != 1 {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid bearer token")
			}
			return next(c)
		}
	}
}

// resetAuthWarnOnceForTests lets the test suite rearm the reminder and stop
// any in-flight ticker goroutine from a previous test.
func resetAuthWarnOnceForTests() {
	noAuthWarnMu.Lock()
	if noAuthWarnStop != nil {
		close(noAuthWarnStop)
		noAuthWarnStop = nil
	}
	noAuthWarnMu.Unlock()
	noAuthWarnOnce = sync.Once{}
	noAuthWarnCount.Store(0)
}
