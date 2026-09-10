#!/usr/bin/env bash
# Generate Parameter Store and Secrets Manager fixtures in a running moto
# server for ad-hoc Clerk exploration.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

MOTO_ENDPOINT="${MOTO_ENDPOINT:-http://localhost:5000}"
NUM_PARAMS="${NUM_PARAMS:-500}"
NUM_SECRETS="${NUM_SECRETS:-50}"

echo "Generating $NUM_PARAMS parameters and $NUM_SECRETS secrets against $MOTO_ENDPOINT..."
MOTO_ENDPOINT="$MOTO_ENDPOINT" NUM_PARAMS="$NUM_PARAMS" \
    go run ./cmd/fixtures \
        -endpoint "$MOTO_ENDPOINT" \
        -count "$NUM_PARAMS" \
        -secrets "$NUM_SECRETS"
