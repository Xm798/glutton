package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
)

func TestEventsHistoryReturnsRecent(t *testing.T) {
	db := newDB(t)
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 100, Level: "info", Type: "x", Message: "hi"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 200, Level: "warn", Type: "y", Message: "ho"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 300, Level: "error", Type: "y", Message: "hum"}))

	srv := api.New(api.Deps{Store: db})

	req := httptest.NewRequest(http.MethodGet, "/api/events/history?limit=10", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got []store.Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 3)
	require.Equal(t, int64(300), got[0].Ts) // DESC order

	req2 := httptest.NewRequest(http.MethodGet, "/api/events/history?since=150", nil)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	var got2 []store.Event
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &got2))
	require.Len(t, got2, 2)

	req3 := httptest.NewRequest(http.MethodGet, "/api/events/history?types=y", nil)
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	var got3 []store.Event
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &got3))
	require.Len(t, got3, 2)
	for _, e := range got3 {
		require.Equal(t, "y", e.Type)
	}
}

// TestM3TypesFilterCrossesLimitBoundary inserts many events of two types
// such that the desired-type rows are pushed past a tight limit window.
// Pre-fix: ListEvents(since, limit) returns only the freshest `limit`
// rows, then in-memory filter throws away everything that isn't the
// requested type → user sees zero rows.
// Post-fix: type filter goes into SQL, so the most recent `limit` rows of
// the requested type are returned.
func TestM3TypesFilterCrossesLimitBoundary(t *testing.T) {
	db := newDB(t)

	// 1..50 = type "noise" (newer ts), 51..55 = type "wanted" (older ts).
	// With limit=10 and a post-limit filter, the API would never reach the
	// "wanted" rows at all.
	for i := 1; i <= 50; i++ {
		require.NoError(t, store.InsertEvent(db, &store.Event{
			Ts: int64(1_700_000_000 + i), Level: "info",
			Type: "noise", Message: "bg",
		}))
	}
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.InsertEvent(db, &store.Event{
			Ts: int64(1_700_000_000 - i), Level: "warn",
			Type: "wanted", Message: "match",
		}))
	}

	srv := api.New(api.Deps{Store: db})
	req := httptest.NewRequest(http.MethodGet, "/api/events/history?types=wanted&limit=10", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got []store.Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 5, "all 5 'wanted' rows must be returned regardless of how many noise rows are newer")
	for _, e := range got {
		require.Equal(t, "wanted", e.Type)
	}
}

// TestM3TypesFilterRespectsLimitWithinType verifies post-filter limit still
// applies — a noisy type with 200 rows + limit=50 returns exactly 50.
func TestM3TypesFilterRespectsLimitWithinType(t *testing.T) {
	db := newDB(t)
	for i := 0; i < 200; i++ {
		require.NoError(t, store.InsertEvent(db, &store.Event{
			Ts: int64(1_700_000_000 + i), Level: "info",
			Type: "wanted", Message: "x",
		}))
	}

	srv := api.New(api.Deps{Store: db})
	req := httptest.NewRequest(http.MethodGet, "/api/events/history?types=wanted&limit=50", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got []store.Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 50)
}

// TestM3MultipleTypesUnion covers `types=a,b` returning rows of either type.
func TestM3MultipleTypesUnion(t *testing.T) {
	db := newDB(t)
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 100, Level: "info", Type: "a"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 200, Level: "info", Type: "b"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 300, Level: "info", Type: "c"}))

	srv := api.New(api.Deps{Store: db})
	req := httptest.NewRequest(http.MethodGet, "/api/events/history?types=a,b", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got []store.Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2)
	seen := map[string]bool{}
	for _, e := range got {
		seen[e.Type] = true
	}
	require.True(t, seen["a"] && seen["b"])
}
