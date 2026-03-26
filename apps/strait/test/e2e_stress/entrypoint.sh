#!/bin/sh
set -e

# Start the built-in echo server in the background.
echoserver &
ECHO_PID=$!

# Wait for echo server to be ready.
sleep 1

# Default echo URL to the local echo server.
export ECHO_BASE_URL="${ECHO_BASE_URL:-http://localhost:9000}"

# Force phase 1 (HTTP scenarios only, no managed execution).
export PHASE=1

# Run the loadtest, forwarding all arguments.
loadtest "$@"
EXIT_CODE=$?

kill $ECHO_PID 2>/dev/null || true
exit $EXIT_CODE
