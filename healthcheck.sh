#!/bin/sh
set -eu

role="${SERVICE_ROLE:-api}"

if [ "$role" = "scheduler" ]; then
  # Verify the scheduler process is running.
  if ps | grep -q "[s]cheduler"; then
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

if curl -fsS "http://$hostport/api/health" >/dev/null; then
  exit 0
fi

echo "api health check failed"
exit 1
