#!/usr/bin/env bash
# SWE302: run every automated test level locally, in the same order as CI.
#   ./test-all.sh        unit -> start system -> integration -> system (E2E)
set -euo pipefail
cd "$(dirname "$0")"
echo "== 1. Unit tests with coverage =="
go test -count=1 -coverprofile=coverage.out ./internal/... ./ui-service/ ./order-service/ ./product-service/
go tool cover -func=coverage.out | tail -1
go tool cover -html=coverage.out -o coverage.html
echo "== 2. Starting the system in test mode =="
./run-all.sh test >/dev/null
trap ./stop-all.sh EXIT
echo "== 3. Integration tests =="
go test -count=1 -v ./tests/integration/
echo "== 4. System (end-to-end) tests =="
python3 -m pytest tests/e2e -v
