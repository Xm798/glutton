# Multi-URL Source — Design

**Date:** 2026-06-02
**Status:** Approved (pending spec review)

## Problem

A logical source can have many download URLs (e.g. Huawei video has 37). Today
`LoadBuiltins()` (`internal/sources/pool.go`) expands a `urls: [...]` group into N
separate `sources` rows named `Huawei video #01` … `#37`. Nothing afterwards knows
those rows belong together, so:

- The source-management page lists all 37 as individual rows (clutter).
- The UI cannot create or edit a source that owns multiple URLs.

## Goal

Make "one source with multiple URLs" a first-class concept across the stack.

Key decisions (settled during brainstorming):

- **Stats live at the source dimension**, not per URL. No per-URL success/fail/speed
  tracking. Cooldown applies to the whole source.
- **URL selection is a simple random pick** within the chosen source. No per-URL
  health probing.
- **URLs stored as a JSON array column** on the `sources` table (no child table).
- **Existing data is discarded** — startup migration wipes `sources` and reseeds.
- **List UI**: one row per source, expandable to view its URL list.
- **Edit UI**: name / UA / weight shared across the source; URLs entered as a
  multi-line textarea (one URL per line).

Non-goals: per-URL health, per-URL weight, dead-URL detection, preserving old rows.

## Data Model (`internal/store`)

`Source.URL string` → `Source.URLs []string`, stored as a JSON-serialized `TEXT`
column. The old `url` column and its `uniqueIndex` are removed.

```go
type Source struct {
    ID            uint     `gorm:"primaryKey;autoIncrement;column:id"`
    Name          string   `gorm:"not null;column:name"`
    URLs          []string `gorm:"serializer:json;not null;column:urls"`
    UA            string   `gorm:"not null;default:'';column:ua"`
    Enabled       bool     `gorm:"not null;default:true;column:enabled"`
    Weight        int      `gorm:"not null;default:1;column:weight"`
    // health stats unchanged — already source-level:
    SuccessCount  int64
    FailCount     int64
    AvgSpeedBps   int64
    LastError     string
    LastSuccessAt int64
    CooldownUntil int64
    CreatedAt     int64
    UpdatedAt     int64
}
```

Stats columns and `traffic_buckets` (keyed by `source_id`) are unchanged — they were
already source-level.

`repo.go`: `SaveSource` updates `urls` instead of `url` in its `Select(...)` column
list. Other repo funcs unaffected.

### Migration

AutoMigrate adds the new `urls` column but will not drop the old `url` column or its
unique index. Since old data is discarded, `store.Open` runs a one-time migration
**before** seeding:

1. Drop the `sources` table (sheds the old `url` column + `uniqueIndex`).
2. AutoMigrate recreates it with the new schema.

Implementation: `db.Migrator().DropTable(&Source{})` guarded so it only fires when the
old `url` column is present (so it's idempotent and a no-op on fresh installs and on
subsequent boots). Reseeding then happens via the existing empty-table path in
`buildSourcePool`.

## Pool / Scheduling (`internal/sources/pool.go`)

- `Candidate.URL string` → `Candidate.URLs []string`.
- `Pick(now, lastID)` still selects a **source** by weight (avoiding `lastID`), then
  picks a **random URL** from `c.URLs`. New signature:
  `Pick(now time.Time, lastID int64) (Candidate, string, bool)` returning the chosen
  candidate, the chosen URL, and ok. The pool owns the rng, so random URL choice lives
  here.
- `LoadBuiltins()` no longer expands groups. `Builtin.URL` → `Builtin.URLs`. A raw
  builtin with `url` (single) normalizes to a one-element `URLs`; one with `urls` maps
  straight through. The `numWidth`/`#NN` naming code is deleted.
- Failure cools down the whole source (existing behavior, unchanged).

## Consumer / Wiring (`cmd/glutton/main.go`)

- Provider: `c, url, ok := sourcePool.Pick(...)`; build `Job{SourceID: uint(c.ID),
  URL: url, UserAgent: pickUA(c.UA, ...)}`. Stats reporting already keys on
  `SourceID` — unchanged.
- `buildSourcePool` and `reloadSourcesFromDB` build `Candidate{URLs: r.URLs, ...}`.
- Seeding: one `store.Source` per builtin carrying its full `URLs` list (no expansion).

## API (`internal/api/sources.go`)

- `sourceIn.URL string` → `URLs []string`.
- Validation: `URLs` must be non-empty; each URL passes `sources.ValidateURL`.
- create/update build `store.Source{URLs: in.URLs, ...}`.
- list/create responses carry `URLs` (GORM JSON output, PascalCase).
- Audit event messages reference URL count instead of a single url.

## Frontend (`web/src`)

- `types/api.ts`: `Source.URL` → `URLs: string[]`; `SourceInput.url` → `urls: string[]`.
- `source-table.tsx`: one row per source. URL column shows a count (e.g. "37 URLs").
  Row is expandable (chevron) to reveal the full URL list (plain list, no per-URL
  stats). Weight / status / edit / delete act on the whole source.
- `source-form-dialog.tsx`: name, UA, weight inputs (shared); a multi-line textarea
  with one URL per line. On submit, split on newlines, trim, drop blanks →
  `urls: string[]`.
- i18n: add keys for "URLs"/url-count label and any new form copy in `en.json` and
  `zh-CN.json`.

## Testing

- `pool_test.go`: weighted source selection still works; `Pick` returns a URL that is a
  member of the chosen source's `URLs`; single-URL sources return their one URL;
  `lastID` avoidance still at source granularity.
- `LoadBuiltins`: `url` and `urls` builtins both produce one Builtin with the right
  `URLs`; no `#NN` expansion.
- store: `SaveSource` round-trips `URLs`; migration drops legacy table when `url`
  column present and is a no-op otherwise.
- `sources_test.go` (api): create/update reject empty `urls`, reject an invalid URL in
  the list, accept multi-URL input.
- Frontend: table renders count + expand; form submits multiple URLs.
