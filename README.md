# Glutton

Self-hosted downstream bandwidth consumer / link exerciser.

Glutton steadily drains downstream bandwidth from a curated pool of public large-file sources on a configurable schedule, discarding the bytes. Useful for:

- Keeping a link warm
- Exercising network paths between hops
- Generating predictable traffic for capacity testing

## Quick start

```bash
docker compose up -d
open http://localhost:7890
```

(Docker image and compose file land in the next plan — for now, build and run locally:)

```bash
make build
GLUTTON_DATA_DIR=./data ./bin/glutton
```

## Configuration

Process-level (env):

| Var | Default | Notes |
|---|---|---|
| `GLUTTON_DATA_DIR` | `/data` | SQLite DB and any state lives here |
| `GLUTTON_LISTEN`   | `:7890` | HTTP listen addr |
| `GLUTTON_LOG_LEVEL`| `info`  | `debug` / `info` / `warn` / `error` |
| `TZ`               | `Asia/Shanghai` | Standard Go timezone string |

Runtime (set via `PUT /api/config` or the web UI):

- `daily_quota_gb`, `monthly_quota_gb` (0 = unlimited)
- `max_rate_mbps` (default 10 — never zero on first install)
- `max_concurrent` (default 4)
- `time_windows` — array of cron expressions, 5-field, minute first (e.g. `["* 0-6 * * *"]` = every minute of hours 0-6)
- `default_ua`, `notifier_urls`, `subscribed_events`

## Security

Glutton has **no built-in authentication** in v1. Bind to LAN only or front it with a reverse-proxy that adds auth.

## Data retention

- Per-source hourly traffic buckets: 30 days
- Audit/event log: 90 days

## License

MIT (see LICENSE).
