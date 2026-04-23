#!/bin/sh
set -eu

scheduler_pid_file="/tmp/scheduler.pid"

# Start scheduler with auto-restart so a crash does not leave the container
# permanently without a worker while the API server keeps running.
start_scheduler() {
	while true; do
		/app/scheduler &
		echo $! > "$scheduler_pid_file"
		wait $!
		echo "scheduler exited (code=$?), restarting in 5s..."
		sleep 5
	done
}

start_scheduler &

terminate() {
	if [ -f "$scheduler_pid_file" ]; then
		kill -TERM "$(cat "$scheduler_pid_file")" 2>/dev/null || true
	fi
}

trap terminate INT TERM

exec /app/server
