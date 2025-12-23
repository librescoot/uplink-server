# Uplink Server

Cloud-side server for librescoot uplink system. Handles bidirectional communication with scooters.

## Features

- **WebSocket-based persistent connections** with compression support
- **Token-based authentication** for scooters and web UI
- **State synchronization** (full snapshots, incremental changes, events)
- **Command sending** to scooters with response tracking
- **Wire-level byte tracking** - monitors actual network bandwidth (post-compression)
- **Real-time web UI** for monitoring and control
- **REST API** for integration and automation
- **Persistent storage** for state and events (survives server restarts)
- **Connection state tracking** with detailed statistics
- **CLI tools** for configuration and client management

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

## Quick Start

### Initialize Configuration

Generate a secure initial configuration with API key:

```bash
./bin/uplink-server init -config config.yml
```

### Add Scooter Client

Generate authentication credentials for a new scooter:

```bash
./bin/uplink-server add-client -config config.yml -identifier WUNU2S3B7MZ000147
```

This outputs a client configuration block to add to your scooter's config.

### Start Server

```bash
./bin/uplink-server -config config.yml
```

Access web UI at `http://localhost:8080` (use the API key from config.yml)

## Configuration

See `configs/config.example.yml` for full configuration reference.

**Key configuration sections:**
- `server.ws_port`: WebSocket port for scooter and web UI connections (default: 8080)
- `server.keepalive_interval`: Keepalive interval (e.g., "5m")
- `auth.api_key`: API key for web UI and REST API access
- `auth.tokens`: Map of scooter identifiers to authentication tokens
- `storage.data_dir`: Directory for persistent state/event storage (default: "./data")
- `logging.stats_interval`: Statistics logging frequency (e.g., "30s")

## Web UI

The server includes a real-time web interface for monitoring and controlling scooters.

**Features:**
- Real-time scooter status and state updates
- Connection statistics with wire-level bandwidth tracking
- Command execution (lock/unlock, open seatbox, hibernate, etc.)
- Event log with dismissal
- Compression ratio display showing bandwidth savings

Access at `http://localhost:8080` (authenticate with API key from config)

## REST API

All endpoints require authentication via `X-API-Key` header.

### Endpoints

**List scooters:**
```bash
GET /api/scooters
```

**Get scooter details:**
```bash
GET /api/scooters/{identifier}
GET /api/scooters/{identifier}/state
GET /api/scooters/{identifier}/events
```

**Send command:**
```bash
POST /api/commands
Content-Type: application/json

{
  "scooter_id": "WUNU2S3B7MZ000147",
  "command": "lock",
  "params": {}
}
```

**Get command result:**
```bash
GET /api/commands/{request_id}
```

## Monitoring

### Connection Statistics

The server tracks both application-level and wire-level statistics:

- **Application bytes**: Uncompressed message data
- **Wire bytes**: Actual network bandwidth (post-compression)
- **Compression ratio**: Bandwidth savings from WebSocket compression
- **Telemetry count**: State/change/event messages received
- **Command count**: Commands sent to scooter

Wire-level tracking uses TCP connection wrappers to count actual bytes transmitted over the network, providing accurate bandwidth usage metrics.

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

## Client Implementation

To connect a scooter to the uplink server:

1. **Establish WebSocket connection** to `ws://server:8080/ws`
2. **Send authentication message** with identifier and token:
   ```json
   {
     "type": "auth",
     "identifier": "WUNU2S3B7MZ000147",
     "token": "your-secret-token",
     "version": "v1.0.0",
     "protocol_version": 1
   }
   ```
3. **Wait for auth response** (`status: "success"`)
4. **Send initial state snapshot** with all current values
5. **Send change messages** as state updates occur
6. **Send event messages** for critical notifications
7. **Handle incoming commands** and send command responses
8. **Respond to keepalive pings** to maintain connection

**WebSocket compression:**
Enable per-message deflate compression for bandwidth savings (typically 20-40% reduction).

**Example clients:**
- librescoot vehicle-service: Reference implementation in Go
- See protocol examples above for message formats

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

[AGPL-3.0](LICENSE)
