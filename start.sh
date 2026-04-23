#!/bin/sh
set -eu

scheduler_pid_file="/tmp/scheduler.pid"

# Start scheduler with auto-restart so a crash does not leave the container
# permanently without a worker while the API server keeps running.
start_scheduler() {
	echo "[start.sh] scheduler restart loop starting"
	while true; do
		# Clean up stale PID file before starting
		rm -f "$scheduler_pid_file"

		/app/scheduler &
		scheduler_pid=$!
		echo $scheduler_pid > "$scheduler_pid_file"
		echo "[start.sh] scheduler started (pid=$scheduler_pid)"

		# Wait for the scheduler to exit and capture its exit code.
		wait_rc=0
		wait $scheduler_pid || wait_rc=$?

		rm -f "$scheduler_pid_file"
		echo "[start.sh] scheduler exited (code=$wait_rc), restarting in 5s..."
		sleep 5
	done
}

start_scheduler &

terminate() {
	echo "[start.sh] received signal, shutting down scheduler..."
	if [ -f "$scheduler_pid_file" ]; then
		kill -TERM "$(cat "$scheduler_pid_file")" 2>/dev/null || true
	fi
	# Also kill the restart loop subshell so it doesn't spawn a new scheduler.
	kill -TERM "$start_scheduler_pid" 2>/dev/null || true
}

trap terminate INT TERM

# Remember the PID of the restart loop subshell.
start_scheduler_pid=$!

echo "[start.sh] starting API server"
exec /app/server
