# syntax=docker/dockerfile:1.7

# ---------- web builder ----------
FROM --platform=$BUILDPLATFORM node:24-alpine AS web-builder
WORKDIR /src/web
RUN npm install -g pnpm@10 --quiet
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---------- go builder ----------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .
COPY --from=web-builder /src/web/dist ./internal/api/spa_dist

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
      -trimpath \
      -ldflags "-s -w \
        -X github.com/cyrus/glutton/internal/version.Version=${VERSION} \
        -X github.com/cyrus/glutton/internal/version.Commit=${COMMIT} \
        -X github.com/cyrus/glutton/internal/version.Date=${DATE}" \
      -o /out/glutton ./cmd/glutton

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=go-builder /out/glutton /usr/local/bin/glutton

USER nonroot:nonroot
ENV GLUTTON_DATA_DIR=/data \
    GLUTTON_LISTEN=:7890 \
    GLUTTON_LOG_LEVEL=info \
    TZ=Asia/Shanghai
EXPOSE 7890
ENTRYPOINT ["/usr/local/bin/glutton"]
