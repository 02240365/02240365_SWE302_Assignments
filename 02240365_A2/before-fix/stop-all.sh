#!/usr/bin/env bash
# Stops everything started by run-all.sh.
cd "$(dirname "$0")"
if [ -f .pids ]; then
  while read -r pid; do kill "$pid" 2>/dev/null || true; done < .pids
  rm -f .pids
fi
# Also free the ports in case a service was started by hand.
for port in 50051 50052 50053 50054 50055 50056 50057 50058 50059 50060 8080; do
  pid=$(lsof -ti tcp:$port 2>/dev/null || true)
  [ -n "$pid" ] && kill $pid 2>/dev/null || true
done
echo "Stopped."
