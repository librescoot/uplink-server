# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pure-Go build (modernc SQLite needs no cgo), stripped.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/uplink-server ./cmd/uplink-server

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
	&& adduser -D -u 10001 uplink
WORKDIR /app

COPY --from=build /out/uplink-server /app/uplink-server
COPY configs /app/configs
COPY docker-entrypoint.sh /app/docker-entrypoint.sh

# data/ holds the SQLite DB, state.json and events.jsonl (mount a volume here).
RUN mkdir -p /app/data && chmod +x /app/docker-entrypoint.sh && chown -R uplink:uplink /app

USER uplink
EXPOSE 8080
VOLUME ["/app/data"]

# CONFIG can be overridden; a config is generated on first run if absent.
ENV CONFIG=/app/data/config.yml
ENTRYPOINT ["/app/docker-entrypoint.sh"]
