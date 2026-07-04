# Uplink Server

Cloud-side server for the librescoot uplink system. Handles bidirectional communication with scooters, stores their telemetry history, and provides a web UI and REST API for monitoring and control.

Part of the [Librescoot](https://librescoot.org/) open-source platform.

## Features

- **WebSocket-based persistent connections** with per-message compression
- **Authentication** — a shared API key **and** username/password login (session tokens)
- **State synchronization** — full snapshots, incremental changes, sparse deltas (with field removals) and batched offline replay
- **Command dispatch** with response tracking and **offline queuing** (safe commands are delivered when the scooter reconnects)
- **Durable persistence (SQLite)** — queryable telemetry history, events, and command history/queue; survives restarts
- **Runtime scooter management** — register/remove scooters from the web UI or API (no CLI edit required)
- **Modern web UI** — flat, responsive, light/dark, with live updates, grouped state, command groups, and historical charts
- **REST API** for integration and automation
- **Wire-level byte tracking** — monitors actual network bandwidth (post-compression)
- **First-run auto-provisioning** — generates a config with a random API key and admin password if none exists
- **Self-contained binary** — the web UI is embedded; no external files needed at runtime

## Requirements

- Go **1.25** or higher (the pure-Go SQLite driver requires it)
- No CGO — builds fully static (`CGO_ENABLED=0`)
- Optional: Docker, for containerized deployment

## Building

```bash
make deps          # download dependencies
make build         # build ./bin/uplink-server

# Cross-compile
make server-linux-amd64
make server-linux-arm
```

Or run directly:

```bash
go build -o bin/uplink-server ./cmd/uplink-server
```

## Quick Start

### 1. Start the server (config is auto-generated)

On first run, if the config file does not exist, the server creates one with a
random API key and a random `admin` password, prints them once, and starts:

```bash
./bin/uplink-server -config config.yml
```

```
================ uplink-server credentials ================
  Config:   config.yml
  API key:  6533428e…c88b2e
  Username: admin
  Password: 696f6277b581305c1df8d404
==========================================================
```

Save those — the password is stored in the config but only shown here once. You
can still pre-generate a config explicitly with `./bin/uplink-server init -config config.yml`.

### 2. Open the web UI

Visit `http://localhost:8080`, click the key icon, and either log in with the
username/password or paste the API key.

### 3. Add a scooter

Use the **+** button in the web UI (enter an identifier, get a ready-to-paste
client config with a one-time token), or the CLI:

```bash
./bin/uplink-server add-client -config config.yml -identifier WUNU2S3B7MZ000147
```

### Docker

```bash
docker compose up --build
# the API key + admin password are printed in the container logs on first run
```

The image is self-contained (the UI is embedded). Use a **named volume** for
`/app/data` (the provided `docker-compose.yml` does) so the config, database, and
state persist and remain writable by the non-root container user.

## Configuration

See `configs/config.example.yml` for the full reference.

**Key sections:**
- `server.ws_port` — port for scooter and web UI connections (default: 8080)
- `server.enable_web_ui` — `true` serves the web UI; `false` runs **API only** (no `/`, `/ws/web`)
- `server.keepalive_interval` — e.g. `"5m"`
- `auth.api_key` — API key for the web UI and REST API
- `auth.tokens` — map of scooter identifier → auth token (managed via the UI/CLI)
- `auth.users` — map of web-UI username → password (omit to disable password login)
- `logging.stats_interval` — statistics logging frequency (e.g. `"30s"`)

Persistent data lives under `./data` (SQLite `uplink.db`, plus `state.json` and
`events.jsonl`).

> **Note:** `auth.api_key` and `auth.users` passwords are stored in plaintext in
> the config file — protect it accordingly.

## Authentication

Two ways to authenticate the web UI and REST API, both presented as
`X-API-Key` (or `?api_key=` for the web WebSocket):

- **API key** — the shared `auth.api_key`.
- **Username/password** — `POST /api/login` with `{username, password}` returns a
  **session token** (24h) that is accepted anywhere the API key is. `POST /api/logout`
  invalidates it.

```bash
curl -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}'
# => {"token":"…","expires_in":86400,"username":"admin"}
```

## Web UI

A modern, self-contained interface embedded in the binary (`enable_web_ui: true`).

- Live scooter status and state updates over `/ws/web`
- **Grouped, collapsible state** panels (Vehicle, Batteries, Location, Powertrain, Connectivity, System) instead of a flat table
- **Grouped command buttons** (Access, Lights, Alarm, Power, Diagnostics) with response feedback
- **Manage Scooters** dialog — add (with a one-time client config to copy) and remove scooters
- **History** dialog — per-scooter charts (speed, battery charge) over selectable ranges
- Username/password **login** or API-key entry
- Automatic **light/dark** theme (follows the OS)
- Connection stats with wire-level bandwidth

Set `enable_web_ui: false` to run headless (API + scooter WebSocket only).

## REST API

All endpoints require authentication (API key or session token via `X-API-Key`),
except `POST /api/login`.

### Scooters & registry

```bash
GET    /api/scooters                     # list connected scooters
POST   /api/scooters                     # register a scooter → returns a token
DELETE /api/scooters/{identifier}        # remove a registered scooter
GET    /api/registry                     # list all registered scooters (+ online flag)

GET    /api/scooters/{id}                # connection details
GET    /api/scooters/{id}/state          # latest state snapshot
GET    /api/scooters/{id}/history?from=&to=&limit=   # persisted telemetry time-series
GET    /api/scooters/{id}/events         # recent events
DELETE /api/scooters/{id}/events         # clear events
DELETE /api/scooters/{id}/events/{eventID}
GET    /api/scooters/{id}/commands       # command history
POST   /api/scooters/{id}/config         # push dotted-path config deltas to the scooter
```

Register a scooter:

```bash
POST /api/scooters
{ "identifier": "WUNU2S3B7MZ000147", "name": "Front Desk" }
# => 201 { "identifier": "...", "name": "...", "token": "…" }
```

### Commands

```bash
POST /api/commands
{
  "scooter_id": "WUNU2S3B7MZ000147",
  "command": "lock",
  "params": {},
  "queue": true,          # optional: queue if the scooter is offline
  "ttl": "1h"             # optional: how long a queued command stays deliverable
}
```

Responses:
- `201 { "request_id": "…", "status": "sent" }` — delivered to an online scooter
- `202 { "request_id": "…", "status": "queued" }` — scooter offline, queued for reconnect
- `404` — scooter not connected (and `queue` not requested)
- `409` — command may not be queued offline (safety: e.g. `unlock`, `open_seatbox`)

The `request_id` (`YYYYMMDD-HHMMSS.microseconds`) tracks the command.

```bash
GET /api/commands/{request_id}

# pending:
{ "request_id": "…", "status": "pending", "message": "Response not yet received" }

# completed:
{ "request_id": "…", "scooter_id": "…", "status": "success",
  "command": "lock", "result": {…}, "received_at": "2026-01-23T15:45:31Z" }
```

Recent command responses are cached in-memory for 1 hour (poll the endpoint);
full command metadata and status also persist in the database.

### Auth

```bash
POST /api/login          # { username, password } → { token, expires_in, username }
POST /api/logout         # invalidates the presented session token
```

## Persistence

The server uses an embedded **SQLite** database (`data/uplink.db`, pure-Go
`modernc.org/sqlite`, no CGO):

- **telemetry_history** — every snapshot (full or post-delta), with extracted
  `lat/lng/speed/state` columns for querying. Exposed via `/api/scooters/{id}/history`.
  Old rows are pruned (default retention 30 days).
- **events** — event log, durable across restarts.
- **commands** — command history that doubles as a **durable, per-scooter queue**:
  offline-queued commands survive restarts, are replayed on reconnect, honor a
  per-command TTL, and never queue physical-actuation commands
  (`unlock`, `open_seatbox`, `force_lock`).

The latest state per scooter is also mirrored to `state.json` for the live dashboard.

## Monitoring

The server tracks both application-level and wire-level statistics:

- **Application bytes** — uncompressed message data
- **Wire bytes** — actual network bandwidth (post-compression), via TCP connection wrappers
- **Compression ratio** — bandwidth savings from WebSocket compression
- **Telemetry / command counts** per connection

## Protocol

### Client → Server

- **auth** — authenticate with identifier and token
- **state** — full state snapshot (nested object structure)
- **change** — incremental field-level changes (nested)
- **telemetry_delta** — changed leaves plus a list of removed dotted paths
- **telemetry_batch** — a batch of buffered offline snapshots (replayed on reconnect)
- **event** — critical event notification
- **keepalive** — keepalive ping
- **command_response** — response to a server command (`status`: `success` / `failed` / `running`)

### Server → Client

- **auth_response** — authentication result
- **command** — execute a command on the scooter
- **keepalive** — keepalive ping
- **config_update** — push dotted-path config deltas (optionally requesting a restart)

### Message format

State and changes use a nested structure keyed by component (Redis hash),
preserving semantic grouping:

```json
{
  "type": "state",
  "timestamp": "2026-01-20T22:45:00Z",
  "data": {
    "battery:0": { "charge": "64", "voltage": "54214" },
    "vehicle":   { "state": "stand-by", "handlebar:lock-sensor": "unlocked" },
    "engine-ecu":{ "speed": "0", "odometer": "1234567" }
  }
}
```

A `telemetry_delta` additionally carries a `removed` list of `hash.field` paths so
the server can prune fields that a merge alone cannot remove:

```json
{
  "type": "telemetry_delta",
  "timestamp": "2026-01-20T22:45:01Z",
  "changes": { "engine-ecu": { "speed": "25" } },
  "removed": ["gps.latitude"]
}
```

## Client Implementation

To connect a scooter:

1. Open a WebSocket to `ws://server:8080/ws`
2. Send an `auth` message with identifier + token; wait for `status: "success"`
3. Send an initial `state` snapshot, then `change`/`telemetry_delta` updates and `event` messages
4. Handle incoming `command` and `config_update` messages; reply with `command_response`
5. Respond to keepalives; enable per-message-deflate compression for bandwidth savings

**Reference client:** [librescoot/uplink-service](https://github.com/librescoot/uplink-service) — Go client for scooters.

## Development

### Project structure

```
uplink-server/
├── cmd/uplink-server/     # main application + CLI subcommands (init, add-client)
├── internal/
│   ├── auth/              # API-key + scooter authentication
│   ├── session/           # username/password login session tokens
│   ├── registry/          # runtime scooter registration + config persistence
│   ├── handlers/          # HTTP / WebSocket handlers (scooter, web UI, REST API)
│   ├── models/            # config + connection data models
│   ├── protocol/          # wire message protocol
│   ├── storage/           # in-memory connection/state/event stores
│   ├── store/             # SQLite persistence (telemetry history, events, commands)
│   └── webui/             # embedded web UI assets (HTML/CSS/JS)
├── configs/               # example configuration
├── Dockerfile, docker-compose.yml
└── bin/                   # built binaries
```

## License

This project is dual-licensed. The source code is available under the
[GNU Affero General Public License v3.0][agpl-3.0].
The maintainers reserve the right to grant separate licenses for commercial distribution; please contact the maintainers to discuss commercial licensing.

[![AGPL v3][agpl-image]][agpl-3.0]

[agpl-3.0]: https://www.gnu.org/licenses/agpl-3.0.en.html
[agpl-image]: https://www.gnu.org/graphics/agplv3-88x31.png
