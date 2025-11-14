#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
FIXTURES_DIR="$PROJECT_ROOT/test/fixtures"

echo "Cleaning up test fixtures from cluster..."

# Check if cluster exists
if ! kubectl cluster-info &>/dev/null; then
    echo "Error: Unable to connect to cluster"
    exit 1
fi

# Delete in reverse order
echo ""
echo "Deleting staging workload..."
kubectl delete -f "$FIXTURES_DIR/04-staging-workload.yaml" --ignore-not-found=true

echo ""
echo "Deleting production workload..."
kubectl delete -f "$FIXTURES_DIR/03-production-workload.yaml" --ignore-not-found=true

echo ""
echo "Deleting safe PVCs and Pods..."
kubectl delete -f "$FIXTURES_DIR/02-safe-pvcs-and-pods.yaml" --ignore-not-found=true

echo ""
echo "Deleting risky PVCs and Pods..."
kubectl delete -f "$FIXTURES_DIR/01-risky-pvcs-and-pods.yaml" --ignore-not-found=true

echo ""
echo "Deleting namespaces..."
kubectl delete -f "$FIXTURES_DIR/00-namespaces.yaml" --ignore-not-found=true

# Wait for namespaces to be fully deleted
echo ""
echo "Waiting for namespaces to be terminated..."
for ns in test-risky test-safe test-production test-staging test-development; do
    if kubectl get namespace "$ns" &>/dev/null; then
        echo "Waiting for namespace $ns to terminate..."
        kubectl wait --for=delete namespace/"$ns" --timeout=120s || true
    fi
done

# Clean up any orphaned PVs
echo ""
echo "Checking for orphaned PVs..."
ORPHANED_PVS=$(kubectl get pv -o json | jq -r '.items[] | select(.status.phase=="Released") | .metadata.name' || true)
if [ -n "$ORPHANED_PVS" ]; then
    echo "Found orphaned PVs (Released state):"
    echo "$ORPHANED_PVS"
    echo ""
    read -p "Do you want to delete these orphaned PVs? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "$ORPHANED_PVS" | xargs -r kubectl delete pv
    fi
fi

echo ""
echo "Cleanup complete!"
echo ""
echo "Remaining PVs:"
kubectl get pv 2>/dev/null || echo "No PVs found"
echo ""
