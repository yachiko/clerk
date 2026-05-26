#!/usr/bin/env bash
# Install moto server locally (alternative to docker-compose).
# Use this when Docker isn't available but Python 3.8+ and pip are.

set -euo pipefail

if ! command -v pip3 >/dev/null 2>&1; then
    echo "error: pip3 is not installed" >&2
    exit 1
fi

echo "Installing moto[server,ssm]..."
pip3 install "moto[server,ssm]>=4.0.0"

echo
echo "Done. Start the server with:"
echo "  moto_server -p 5000"
echo
echo "Then run integration tests with:"
echo "  MOTO_ENDPOINT=http://localhost:5000 go test -tags=integration ./internal/integration/..."
