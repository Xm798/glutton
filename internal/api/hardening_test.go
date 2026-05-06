package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/stretchr/testify/require"
)

func TestM2SecureHeadersPresent(t *testing.T) {
	srv := api.New(api.Deps{})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	h := rec.Header()
	require.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	require.Equal(t, "SAMEORIGIN", h.Get("X-Frame-Options"))
	require.NotEmpty(t, h.Get("Content-Security-Policy"))
	require.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
}

func TestM2BodyLimitReturns413(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	// 4 MB body is well over the 1 MB cap.
	big := strings.Repeat("x", 4*1024*1024)
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"x":"`+big+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}

func TestM2CORSDefaultClosed(t *testing.T) {
	t.Setenv("GLUTTON_CORS_ORIGINS", "")
	srv := api.New(api.Deps{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/version", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	srv.ServeHTTP(rec, req)
	// Without CORS middleware installed, no Access-Control-Allow-Origin header is set.
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"),
		"closed-by-default CORS: no Allow-Origin echoed back")
}

func TestM2CORSEnvOptIn(t *testing.T) {
	t.Setenv("GLUTTON_CORS_ORIGINS", "https://allowed.example")
	srv := api.New(api.Deps{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/version", nil)
	req.Header.Set("Origin", "https://allowed.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	srv.ServeHTTP(rec, req)
	require.Equal(t, "https://allowed.example", rec.Header().Get("Access-Control-Allow-Origin"))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/api/version", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	srv.ServeHTTP(rec, req)
	// Echo's CORS middleware leaves the header empty for non-allowed origins.
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestM2AuthEnforcedThroughServer(t *testing.T) {
	t.Setenv("GLUTTON_AUTH_TOKEN", "supersecret")
	srv := api.New(api.Deps{})

	// /api/sources requires auth now.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sources", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "/api/version is always public")

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "/metrics is always public for Prometheus scrape")
}
