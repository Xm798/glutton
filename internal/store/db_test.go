package store_test

import (
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(db) })
	return db
}

func TestOpenAutoMigratesAllTables(t *testing.T) {
	db := openTestDB(t)
	for _, tbl := range []string{"config_kv", "sources", "traffic_buckets", "events"} {
		require.True(t, db.Migrator().HasTable(tbl), "missing table %s", tbl)
	}
}

func TestUpsertAndGetConfig(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, store.UpsertConfig(db, "daily_quota_gb", "100"))
	v, err := store.GetConfig(db, "daily_quota_gb")
	require.NoError(t, err)
	require.Equal(t, "100", v)

	require.NoError(t, store.UpsertConfig(db, "daily_quota_gb", "200"))
	v, err = store.GetConfig(db, "daily_quota_gb")
	require.NoError(t, err)
	require.Equal(t, "200", v)

	all, err := store.ListConfig(db)
	require.NoError(t, err)
	require.Equal(t, "200", all["daily_quota_gb"])
}

func TestSourceCRUDAndHealth(t *testing.T) {
	db := openTestDB(t)

	s := &store.Source{Name: "h", URLs: []string{"https://example.com/100MB.bin"}, Weight: 3, Enabled: true}
	require.NoError(t, store.CreateSource(db, s))
	require.NotZero(t, s.ID)
	require.NotZero(t, s.CreatedAt)

	got, err := store.ListEnabledSources(db)
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.NoError(t, store.RecordSourceSuccess(db, s.ID, 1_000_000, 1_700_000_000))
	require.NoError(t, store.RecordSourceFailure(db, s.ID, "timeout", 1_700_000_060))

	var refreshed store.Source
	require.NoError(t, db.First(&refreshed, s.ID).Error)
	require.Equal(t, int64(1), refreshed.SuccessCount)
	require.Equal(t, int64(1), refreshed.FailCount)
	require.Equal(t, "timeout", refreshed.LastError)
	require.Equal(t, int64(1_700_000_060), refreshed.CooldownUntil)
}

func TestAddTrafficUpsertAccumulates(t *testing.T) {
	db := openTestDB(t)
	s := &store.Source{Name: "h", URLs: []string{"https://example.com/x"}, Weight: 1, Enabled: true}
	require.NoError(t, store.CreateSource(db, s))

	const bucket int64 = 1_700_000_000
	require.NoError(t, store.AddTraffic(db, bucket, s.ID, 100))
	require.NoError(t, store.AddTraffic(db, bucket, s.ID, 250))

	rows, err := store.TrafficSinceBucket(db, bucket)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(350), rows[0].Bytes)
}

func TestTrafficBySourceJoin(t *testing.T) {
	db := openTestDB(t)
	a := &store.Source{Name: "a", URLs: []string{"https://example.com/a"}, Weight: 1, Enabled: true}
	b := &store.Source{Name: "b", URLs: []string{"https://example.com/b"}, Weight: 1, Enabled: true}
	require.NoError(t, store.CreateSource(db, a))
	require.NoError(t, store.CreateSource(db, b))

	require.NoError(t, store.AddTraffic(db, 1_000, a.ID, 500))
	require.NoError(t, store.AddTraffic(db, 1_000, b.ID, 100))

	rows, err := store.TrafficBySource(db, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "a", rows[0].Name)
	require.Equal(t, int64(500), rows[0].Bytes)
}

func TestEventsInsertListPurge(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 100, Level: "info", Type: "x", Message: "hi"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{Ts: 200, Level: "warn", Type: "y", Message: "ho"}))

	all, err := store.ListEvents(db, 0, 10)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, int64(200), all[0].Ts)

	require.NoError(t, store.PurgeEventsBefore(db, 150))
	remaining, err := store.ListEvents(db, 0, 10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	require.Equal(t, int64(200), remaining[0].Ts)
}

func TestSaveSourceDoesNotClobberHealthStats(t *testing.T) {
	db := openTestDB(t)
	s := &store.Source{Name: "h", URLs: []string{"https://example.com/x"}, Weight: 1, Enabled: true}
	require.NoError(t, store.CreateSource(db, s))

	// Consumer records a success.
	require.NoError(t, store.RecordSourceSuccess(db, s.ID, 5_000_000, 1_700_000_000))

	// Operator edits the name via a stale in-memory copy.
	stale := *s // health counters still zero in this copy
	stale.Name = "renamed"
	require.NoError(t, store.SaveSource(db, &stale))

	// Reload and verify health stats survived.
	var got store.Source
	require.NoError(t, db.First(&got, s.ID).Error)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, int64(1), got.SuccessCount)
	require.Equal(t, int64(5_000_000), got.AvgSpeedBps)
	require.Equal(t, int64(1_700_000_000), got.LastSuccessAt)
}

func TestListEventsFilteredPushesTypePredicateBelowLimit(t *testing.T) {
	db := openTestDB(t)

	for i := 1; i <= 30; i++ {
		require.NoError(t, store.InsertEvent(db, &store.Event{
			Ts: int64(1000 + i), Level: "info", Type: "noise", Message: "x",
		}))
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, store.InsertEvent(db, &store.Event{
			Ts: int64(900 - i), Level: "warn", Type: "wanted", Message: "y",
		}))
	}

	rows, err := store.ListEventsFiltered(db, 0, 5, []string{"wanted"})
	require.NoError(t, err)
	require.Len(t, rows, 3, "all 3 'wanted' rows must come back even though 30 newer 'noise' rows exist")
	for _, r := range rows {
		require.Equal(t, "wanted", r.Type)
	}

	// Empty slice == no filter.
	all, err := store.ListEventsFiltered(db, 0, 100, nil)
	require.NoError(t, err)
	require.Len(t, all, 33)
}

func TestMinuteBucketAlignment(t *testing.T) {
	ts := time.Date(2026, 6, 3, 13, 4, 39, 0, time.UTC)
	require.Equal(t, time.Date(2026, 6, 3, 13, 4, 0, 0, time.UTC).Unix(), store.MinuteBucket(ts))
}

func TestOpenAutoMigratesMinuteSamples(t *testing.T) {
	db := openTestDB(t)
	require.True(t, db.Migrator().HasTable("minute_samples"))
}

func TestMinuteSampleUpsertQueryPurge(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, store.AddMinuteSample(db, 1_000_060, 100))
	require.NoError(t, store.AddMinuteSample(db, 1_000_060, 250)) // same bucket accumulates
	require.NoError(t, store.AddMinuteSample(db, 1_000_120, 70))

	rows, err := store.MinuteSamplesSince(db, 1_000_060)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(1_000_060), rows[0].MinuteBucket)
	require.Equal(t, int64(350), rows[0].Bytes)
	require.Equal(t, int64(70), rows[1].Bytes)

	// since filter excludes older buckets
	rows, err = store.MinuteSamplesSince(db, 1_000_120)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// purge removes buckets strictly before the cutoff
	require.NoError(t, store.PurgeMinuteSamplesBefore(db, 1_000_120))
	rows, err = store.MinuteSamplesSince(db, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1_000_120), rows[0].MinuteBucket)
}

func TestMaxEventIDOnEmptyAndPopulated(t *testing.T) {
	db := openTestDB(t)

	got, err := store.MaxEventID(db)
	require.NoError(t, err)
	require.Equal(t, uint64(0), got)

	require.NoError(t, store.InsertEvent(db, &store.Event{EventID: 7, Ts: 1, Level: "info", Type: "x", Message: "m"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{EventID: 99, Ts: 2, Level: "info", Type: "x", Message: "m"}))
	require.NoError(t, store.InsertEvent(db, &store.Event{EventID: 42, Ts: 3, Level: "info", Type: "x", Message: "m"}))

	got, err = store.MaxEventID(db)
	require.NoError(t, err)
	require.Equal(t, uint64(99), got)
}
