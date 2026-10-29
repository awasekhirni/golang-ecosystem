# Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
# Author: Awase Khirni Syed
#
# Multi-stage Dockerfile.
#   stage 1 (builder): static build with -trimpath -ldflags "-s -w"
#   stage 2 (runtime): distroless / non-root, ~20MB image
#
# GAP CLOSED: #16 Dockerfile + docker-compose (Containerization)

# --- builder ----------------------------------------------------------------
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Cache deps first (only re-runs when go.mod/go.sum change).
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build static binaries.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o /out/generator ./cmd/generator

# --- runtime ----------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy binaries and migrations.
COPY --from=builder /out/server     /app/server
COPY --from=builder /out/migrate    /app/migrate
COPY --from=builder /out/generator  /app/generator
COPY --from=builder /src/internal/database/migrations /app/migrations

# Non-root user (already provided by distroless:nonroot).
USER nonroot:nonroot

EXPOSE 8080

# Health probes (Kubernetes reads these).
HEALTHCHECK --interval=10s --timeout=2s --retries=3 \
  CMD ["/app/server", "-probe"]

ENTRYPOINT ["/app/server"]
