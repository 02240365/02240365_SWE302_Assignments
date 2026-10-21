#!/usr/bin/env bash
# Starts the ten gRPC services and the gateway/UI, each in the background.
#   ./run-all.sh          normal start
#   ./run-all.sh test     also switches on POST /api/test/reset (needed by Cypress)
# Logs go to .logs/<service>.log. Stop everything with ./stop-all.sh
# Without LAMBDA_FUNCTION_NAME the gateway uses the same discount rules locally.
# To use the real AWS Lambda, export AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
# AWS_REGION and LAMBDA_FUNCTION_NAME before running this script.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p .logs .bin
[ "${1:-}" = "test" ] && export ENABLE_TEST_ENDPOINTS=1 && echo "Test mode: /api/test/reset is ON"

SERVICES="product-service inventory-service customer-service payment-service notification-service cart-service review-service shipping-service coupon-service history-service"
./stop-all.sh >/dev/null 2>&1 || true

echo "Building..."
for s in $SERVICES ui-service; do go build -o ".bin/$s" "./$s"; done

: > .pids
for s in $SERVICES ui-service; do
  ".bin/$s" > ".logs/$s.log" 2>&1 &
  echo $! >> .pids
  echo "started $s (pid $!)"
done

echo "Waiting for the gateway on http://localhost:8080 ..."
for i in $(seq 1 30); do
  if curl -fsS http://localhost:8080/api/services >/dev/null 2>&1; then
    echo "Ready. Open http://localhost:8080"
    curl -s http://localhost:8080/api/services; echo
    exit 0
  fi
  sleep 1
done
echo "The gateway did not start. Look in .logs/" >&2
exit 1
