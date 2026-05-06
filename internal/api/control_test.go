package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/stretchr/testify/require"
)

func TestControlPauseResume(t *testing.T) {
	st := scheduler.NewState()
	srv := api.New(api.Deps{State: st})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/control/pause", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, scheduler.Paused, st.Get())

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/control/resume", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, scheduler.Idle, st.Get())
}

func TestControlStatusEndpoint(t *testing.T) {
	st := scheduler.NewState()
	_ = st.Activate()
	srv := api.New(api.Deps{State: st})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/control/status", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "running", got["status"])
	require.Equal(t, false, got["burst_active"])
}

type recordingBurst struct{ n atomic.Int32 }

func (r *recordingBurst) Burst(_ int, _ int64) { r.n.Add(1) }

func TestControlBurstReturns409WhenQuotaReached(t *testing.T) {
	st := scheduler.NewState()
	_ = st.QuotaReached()
	rb := &recordingBurst{}
	srv := api.New(api.Deps{State: st, Burst: rb})

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"minutes":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/control/burst", body)
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, int32(0), rb.n.Load())
}

func TestControlBurstSucceedsWhenIdle(t *testing.T) {
	st := scheduler.NewState()
	rb := &recordingBurst{}
	srv := api.New(api.Deps{State: st, Burst: rb})

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"minutes":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/control/burst", body)
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, int32(1), rb.n.Load())
}
