#!/bin/bash
# Run uplink-server locally

cd "$(dirname "$0")"

if [ ! -f bin/uplink-server ]; then
    echo "Binary not found. Building..."
    go build -o bin/uplink-server ./cmd/uplink-server
fi

LOGDIR="logs"
LOGFILE="$LOGDIR/uplink-server.log"

mkdir -p "$LOGDIR"

echo "Starting uplink-server on :8080..."
echo "Logs: $LOGFILE"
echo ""
echo "Press Ctrl+C to stop"
echo ""

./bin/uplink-server -config config-local.yml 2>&1 | tee -a "$LOGFILE"
