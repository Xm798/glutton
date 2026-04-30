package api_test

import (
	"net/http"
	"net/http/httptest"
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
	// Resume from Paused with no prior Activate returns to Idle (per scheduler State semantics).
	require.Equal(t, scheduler.Idle, st.Get())
}
