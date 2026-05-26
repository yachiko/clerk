#!/usr/bin/env bash
# Run the integration suite against a moto server. Brings moto up via
# docker-compose when Docker is available, otherwise expects a moto_server
# already running at MOTO_ENDPOINT (default http://localhost:5000).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

MOTO_ENDPOINT="${MOTO_ENDPOINT:-http://localhost:5000}"
USE_DOCKER="${USE_DOCKER:-auto}"

started_docker=0

cleanup() {
    if [[ "$started_docker" == "1" ]]; then
        echo "=== Stopping moto (docker) ==="
        docker compose -f docker-compose.test.yml down >/dev/null 2>&1 || \
            docker-compose -f docker-compose.test.yml down >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

if [[ "$USE_DOCKER" == "auto" ]]; then
    if command -v docker >/dev/null 2>&1; then
        USE_DOCKER=1
    else
        USE_DOCKER=0
    fi
fi

if [[ "$USE_DOCKER" == "1" ]]; then
    echo "=== Starting moto via docker compose ==="
    docker compose -f docker-compose.test.yml up -d >/dev/null 2>&1 || \
        docker-compose -f docker-compose.test.yml up -d
    started_docker=1
fi

echo "=== Waiting for moto at $MOTO_ENDPOINT ==="
for i in $(seq 1 30); do
    if curl -sf "$MOTO_ENDPOINT/moto-api/" >/dev/null 2>&1; then
        echo "moto is ready"
        break
    fi
    sleep 1
    if [[ "$i" == "30" ]]; then
        echo "error: moto server not reachable at $MOTO_ENDPOINT" >&2
        echo "       start it manually with: moto_server -p 5000" >&2
        exit 1
    fi
done

echo "=== Building clerk ==="
go build -o ./bin/clerk ./cmd/clerk

echo "=== Running integration tests ==="
MOTO_ENDPOINT="$MOTO_ENDPOINT" go test -v -tags=integration ./internal/integration/...
