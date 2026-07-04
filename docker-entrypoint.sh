#!/bin/sh
set -e

# The server auto-generates the config (with a random API key and admin
# password) on first run if it does not exist, printing the credentials to the
# log. Use a named volume for /app/data so it persists and is writable.
exec /app/uplink-server -config "${CONFIG:-/app/data/config.yml}"
