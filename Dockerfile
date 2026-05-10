# syntax=docker/dockerfile:1.7

# ─── Stage 1: Build ───────────────────────────────────────────────────────────
# Use the full Go toolchain image. Pin to a specific minor for reproducibility.
FROM golang:1.25-alpine AS builder

# Install git (required by some Go modules that reference VCS)
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Copy dependency manifests first — cached as long as go.mod/go.sum don't change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

# Copy source and build a fully static binary.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w -extldflags '-static'" \
      -trimpath \
      -o /out/tele-trader \
      ./cmd/bot/...

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
# Distroless "static" has no shell, no package manager, minimal attack surface.
# It includes CA certs so HTTPS calls to Anthropic/OpenAlgo work out of the box.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

# Copy timezone data and CA certs from builder (already in alpine).
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the compiled binary.
COPY --from=builder /out/tele-trader /tele-trader

# Run as non-root (UID 65532 = "nonroot" in distroless).
USER nonroot:nonroot

# The bot is a long-running daemon — no ports to expose.
# session.json is stored in /data (a named volume mounted at runtime).
VOLUME ["/data"]

ENTRYPOINT ["/tele-trader"]
