#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Building pv-safe webhook..."
cd "$PROJECT_ROOT"

echo "Building Go binary locally..."
go build -o bin/webhook cmd/webhook/main.go
echo "Binary built: bin/webhook"

echo ""
echo "Building Docker image for kind..."
docker build -t pv-safe-webhook:latest .

echo ""
echo "Loading image into kind cluster..."
kind load docker-image pv-safe-webhook:latest --name pv-safe-test

echo ""
echo "Webhook image built and loaded successfully!"
echo ""
echo "To deploy:"
echo "  make webhook-deploy"
