# Uplink Server

Cloud-side server for librescoot uplink system. Handles bidirectional communication with scooters.

## Features

- WebSocket-based persistent connections
- Token-based authentication
- State synchronization (full snapshots, incremental changes, events)
- Command sending to scooters
- Connection state tracking
- Bandwidth and statistics monitoring
- Configurable keepalive intervals

## Requirements

- Go 1.22 or higher

## Building

```bash
# Download dependencies
make deps

# Build server
make build

# Build for specific platforms
make server-linux-amd64
make server-linux-arm
```

## Configuration

Create a `config.yml` file (see `configs/config.example.yml`):

```yaml
server:
  ws_port: 8080
  lp_port: 8081
  sse_port: 8082
  keepalive_interval: "5m"
  lp_timeout: "24h"

auth:
  tokens:
    "mdb-12345678": "secret-token-here"

storage:
  type: "memory"

logging:
  level: "info"
  stats_interval: "30s"
```

## Running

```bash
./bin/uplink-server -config config.yml
```

## Protocol

### Client → Server Messages

- **auth**: Authenticate with identifier and token
- **state**: Send full state snapshot (nested object structure)
- **change**: Send incremental field-level changes (nested object structure)
- **event**: Send critical event notification
- **keepalive**: Keepalive ping
- **command_response**: Response to server command

### Server → Client Messages

- **auth_response**: Authentication result
- **command**: Execute command on scooter
- **keepalive**: Keepalive ping
- **config_update**: Update configuration

### Message Format Details

**State Message** (full snapshot with nested objects):
```json
{
  "type": "state",
  "timestamp": "2025-12-20T22:45:00Z",
  "data": {
    "battery:0": {
      "charge": "64",
      "voltage": "54214",
      "current": "-190"
    },
    "vehicle": {
      "state": "stand-by",
      "handlebar:lock-sensor": "unlocked"
    },
    "engine-ecu": {
      "speed": "0",
      "odometer": "1234567"
    }
  }
}
```

**Change Message** (incremental updates with nested objects):
```json
{
  "type": "change",
  "timestamp": "2025-12-20T22:45:01Z",
  "changes": {
    "battery:0": {
      "charge": "65",
      "current": "-180"
    },
    "engine-ecu": {
      "speed": "25"
    }
  }
}
```

**Benefits of nested structure:**
- Consistent format across all message types
- Preserves semantic grouping (battery:0, vehicle, engine-ecu)
- Easier querying - access all battery or vehicle data directly
- Cleaner data model matching logical system organization

## Architecture

```
┌─────────────────┐
│  HTTP Handler   │
│   (WebSocket)   │
└────────┬────────┘
         │
    ┌────▼────┐
    │  Auth   │
    └────┬────┘
         │
┌────────▼────────┐
│  Connection Mgr │
│   (Storage)     │
└─────────────────┘
```

## Development

### Project Structure

```
uplink-server/
├── cmd/uplink-server/    # Main application
├── internal/
│   ├── auth/             # Authentication
│   ├── handlers/         # HTTP/WebSocket handlers
│   ├── models/           # Data models
│   ├── protocol/         # Message protocol
│   └── storage/          # Connection management
├── configs/              # Example configurations
└── bin/                  # Built binaries
```

## License

AGPL-3.0 (matches librescoot project)
