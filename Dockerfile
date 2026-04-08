# syntax=docker/dockerfile:1.7
#
# AI Coop DB server image — multi-stage, multi-arch (amd64 + arm64), distroless.
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 -t ai-coop-db-server:dev .
#
# The final image:
#   - runs as uid 65532 (nonroot, distroless convention)
#   - has no shell, no apt, no busybox
#   - read-only root filesystem friendly (no writes outside /tmp)
#   - ARG TARGETARCH-aware so buildx slices the right Go binary in
#
# Migrations are embedded in the binary via cmd/server's call to db.RunMigrations,
# which uses the migrations/ files baked into the image at /app/migrations.

# GO_VERSION must be >= the `go` directive in go.mod (currently 1.25, raised
# transitively by go.opentelemetry.io/otel v1.43). Bump in lockstep when
# `go mod tidy` raises the directive again.
ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.21

# ---- builder -----------------------------------------------------------------
#
# pg_query_go embeds the PostgreSQL C parser and REQUIRES cgo. We therefore:
#   - install gcc + musl-dev so cgo can compile the C sources
#   - set CGO_ENABLED=1
#   - link statically (-extldflags "-static") so the resulting binary still
#     runs on distroless/static (no glibc, no musl loader at runtime)
#
# We deliberately do NOT use --platform=$BUILDPLATFORM here. Cgo cross-
# compilation needs a cross-toolchain (xx, gcc-aarch64-linux-musl-cross, ...);
# letting buildx run the builder under QEMU for each TARGETPLATFORM is slower
# but keeps the Dockerfile simple. CI's `buildx` job uses
# `docker/setup-qemu-action` which provides the binfmt handlers.

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

ARG VERSION=0.1.0-dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

ENV CGO_ENABLED=1 \
    GO111MODULE=on \
    GOPROXY=https://proxy.golang.org,direct

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

# Cache module downloads in their own layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath \
      -ldflags "-s -w -extldflags '-static' \
        -X github.com/fheinfling/ai-coop-db/internal/version.Version=${VERSION} \
        -X github.com/fheinfling/ai-coop-db/internal/version.Commit=${COMMIT} \
        -X github.com/fheinfling/ai-coop-db/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/ai-coop-db-server ./cmd/server && \
    go build -trimpath \
      -ldflags "-s -w -extldflags '-static'" \
      -o /out/ai-coop-db-migrate ./cmd/migrate

# ---- runtime -----------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="ai-coop-db-server" \
      org.opencontainers.image.source="https://github.com/fheinfling/ai-coop-db" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.description="Auth gateway for shared PostgreSQL"

WORKDIR /app

COPY --from=builder /out/ai-coop-db-server /app/ai-coop-db-server
COPY --from=builder /out/ai-coop-db-migrate /app/ai-coop-db-migrate
COPY migrations /app/migrations
COPY sql /app/sql

USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/app/ai-coop-db-server"]
