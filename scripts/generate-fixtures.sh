#!/usr/bin/env bash
# Generate N random parameters in a running moto server, useful for ad-hoc
# manual exploration of clerk against a populated Parameter Store.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

MOTO_ENDPOINT="${MOTO_ENDPOINT:-http://localhost:5000}"
NUM_PARAMS="${NUM_PARAMS:-500}"

echo "Generating $NUM_PARAMS parameters against $MOTO_ENDPOINT..."
MOTO_ENDPOINT="$MOTO_ENDPOINT" NUM_PARAMS="$NUM_PARAMS" \
    go run ./cmd/fixtures \
        -endpoint "$MOTO_ENDPOINT" \
        -count "$NUM_PARAMS"
