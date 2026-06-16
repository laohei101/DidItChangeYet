# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Cache module downloads separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /http-watcher .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM scratch

COPY --from=builder /http-watcher /http-watcher
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Config and state are mounted from the host
VOLUME ["/data"]
WORKDIR /data

EXPOSE 8080

ENTRYPOINT ["/http-watcher"]
CMD ["--config", "/data/config.yaml"]
