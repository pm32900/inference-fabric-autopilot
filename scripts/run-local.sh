#!/usr/bin/env bash
# run-local.sh - starts the control plane locally for deployment

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
echo  "==> Building control plane..."
go build -o "$PROJECT_ROOT/bin/control-plane" "$PROJECT_ROOT/cmd/control-plane"

echo "==> Starting control plane on http://localhost:8080"
echo "    Endpoints: GET /healthz GET /telemetry GET /recommendations"
echo "    Press Ctrl+C to stop."
echo ""

$PROJECT_ROOT/bin/control-plane

