#!/bin/sh
set -eu

role="${SERVICE_ROLE:-api}"
scheduler_pid_file="/tmp/scheduler.pid"

# Check if a process with the given PID is alive.
is_pid_alive() {
  kill -0 "$1" 2>/dev/null
}

# Check if the scheduler process is running.
# Prefers the PID file (most reliable); falls back to ps.
scheduler_is_running() {
  if [ -f "$scheduler_pid_file" ]; then
    pid=$(cat "$scheduler_pid_file")
    if [ -n "$pid" ] && is_pid_alive "$pid"; then
      return 0
    fi
  fi

  # Fallback: ps check. Use 'ps w' for wide output in BusyBox.
  if ps w 2>/dev/null | grep -q "[s]cheduler"; then
    return 0
  fi
  if ps 2>/dev/null | grep -q "[s]cheduler"; then
    return 0
  fi

  return 1
}

if [ "$role" = "scheduler" ]; then
  if scheduler_is_running; then
    exit 0
  fi
  echo "scheduler process not running"
  exit 1
fi

addr="${HTTP_ADDR:-:8080}"
case "$addr" in
  :*) hostport="127.0.0.1$addr" ;;
  0.0.0.0:*) hostport="127.0.0.1:${addr#0.0.0.0:}" ;;
  *) hostport="$addr" ;;
esac

# In the combined container both API and scheduler must be healthy.
if ! scheduler_is_running; then
  # Provide extra diagnostics for operators
  if [ -f "$scheduler_pid_file" ]; then
    pid=$(cat "$scheduler_pid_file")
    echo "scheduler process not running (pid file exists: $pid)"
  else
    echo "scheduler process not running (no pid file)"
  fi
  exit 1
fi

if curl -fsS "http://$hostport/api/health" >/dev/null; then
  exit 0
fi

echo "api health check failed"
exit 1
