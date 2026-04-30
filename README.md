# Glutton

Self-hosted downstream bandwidth consumer / link exerciser.

Glutton steadily drains downstream bandwidth from a curated pool of public large-file sources on a configurable schedule, discarding the bytes. Useful for:

- Keeping a link warm
- Exercising network paths between hops
- Generating predictable traffic for capacity testing

## Quick start

### Docker (recommended)

```bash
mkdir -p data
# Linux: sudo chown 65532:65532 data    (distroless nonroot uid)
# macOS Docker Desktop: chmod 777 data  (VM handles uid mapping)

docker run -d --name glutton \
  --restart unless-stopped \
  -p 7890:7890 \
  -v $(pwd)/data:/data \
  -e TZ=Asia/Shanghai \
  ghcr.io/<owner>/glutton:latest

open http://localhost:7890
```

Or with compose:

```bash
docker compose up -d
```

(Edit the `image:` line in `docker-compose.yml` to point at your published image, then run.)

### Build from source

```bash
make build
GLUTTON_DATA_DIR=./data ./bin/glutton
```

`make build` runs `pnpm install && pnpm build` (web SPA) and the Go compile. Requires Go 1.26+, Node 24+, and pnpm 10+.

### Build a local Docker image

```bash
docker build \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t glutton:dev .
```

## Image

- Base: `gcr.io/distroless/static-debian12:nonroot` (uid 65532)
- Size: ~19 MB
- Architectures: `linux/amd64`, `linux/arm64`
- Pure-Go SQLite — no CGO, no glibc dependency

## Configuration

Process-level (env):

| Var | Default | Notes |
|---|---|---|
| `GLUTTON_DATA_DIR` | `./data` | SQLite DB and any state lives here (Docker compose sets this to `/data`) |
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
