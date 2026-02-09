#!/bin/sh
set -eu

# Start scheduler in background and forward termination signals.
/app/scheduler &
scheduler_pid=$!

terminate() {
	kill -TERM "$scheduler_pid" 2>/dev/null || true
}

trap terminate INT TERM

exec /app/server
