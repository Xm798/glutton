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
| `GLUTTON_VALIDATE_DNS_TIMEOUT_MS` | `3000` | Per-source URL validation DNS lookup deadline. Lower (e.g. `1000`) for internal-only deployments; raise for slow international DNS. |
| `TZ`               | `Asia/Shanghai` | Standard Go timezone string |

Runtime (set via `PUT /api/config` or the web UI):

- `daily_quota_gb`, `monthly_quota_gb` (0 = unlimited)
- `max_rate_mbps` (default 10; 0 = unlimited)
- `max_concurrent` (default 4)
- `time_windows` — array of cron expressions, 5-field, minute first (e.g. `["* 0-6 * * *"]` = every minute of hours 0-6)
- `default_ua`, `notifier_urls`, `subscribed_events`

## Security & production deployment checklist

Glutton's HTTP surface is intended for trusted networks. Before exposing the
service publicly, do **all** of the following:

1. **Set a token**. Export `GLUTTON_AUTH_TOKEN=<long random string>`. The
   `/api/*` routes will then require `Authorization: Bearer <token>`.
   `/metrics` and `/api/version` stay public so Prometheus and health probes
   keep working. With the variable unset, the server logs a one-shot WARN
   on the first API request.
2. **Front with TLS**. Put a reverse-proxy (Caddy, nginx, Traefik) in front
   to terminate HTTPS. The Go process speaks plain HTTP on `:7890`.
3. **Keep CORS closed unless you need it**. Cross-origin is disabled by
   default. To run the SPA from a different host, set
   `GLUTTON_CORS_ORIGINS=https://app.example.com,https://www.example.com`
   (comma-separated; `*` works but is not recommended).
4. **Bind to localhost when possible**. The compose file uses
   `network_mode: host`; if you switch to a published port, prefer
   `127.0.0.1:7890:7890` and let the proxy forward.
5. **Source URL hygiene is enforced**. The validator rejects loopback,
   RFC1918, link-local, CGNAT (100.64/10), and cloud metadata IPs
   (`169.254.169.254`, `100.100.100.200`). DNS hostnames are resolved at
   validation time and refused if any answer falls in those ranges; the
   actual download dialer re-checks the resolved peer at SYN time so a
   post-validation DNS rebind still fails.

The built-in security headers (X-Content-Type-Options, X-Frame-Options,
Content-Security-Policy, Referrer-Policy) and a 1 MiB request-body cap are
always on; no opt-in needed.

If you only run Glutton on a trusted LAN, skipping (1) is acceptable but
the WARN log line will tell you each time the process restarts.

## Data retention

- Per-source hourly traffic buckets: 30 days
- Audit/event log: 90 days

## License

MIT (see LICENSE).
