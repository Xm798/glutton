package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/events"
	"github.com/stretchr/testify/require"
)

func TestM4EventTypesEndpointReturnsAllKnown(t *testing.T) {
	srv := api.New(api.Deps{})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events/types", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		All     []string `json:"all"`
		Default []string `json:"default"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	// Must include every type defined in events package, including cooldown.
	require.Contains(t, got.All, events.TypeSourceCooldown)
	require.Contains(t, got.All, events.TypeQuotaReachedDaily)
	require.Contains(t, got.All, events.TypeStateChanged)
	require.Contains(t, got.Default, events.TypeSourceCooldown,
		"M4: source_cooldown must be in default subscriptions on fresh install")

	// Default must be a subset of All.
	all := map[string]bool{}
	for _, t := range got.All {
		all[t] = true
	}
	for _, d := range got.Default {
		require.True(t, all[d], "default type %q must appear in 'all'", d)
	}
}
