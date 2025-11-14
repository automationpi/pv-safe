#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
FIXTURES_DIR="$PROJECT_ROOT/test/fixtures"

echo "Applying test fixtures to cluster..."

# Check if cluster exists
if ! kubectl cluster-info &>/dev/null; then
    echo "Error: Unable to connect to cluster"
    echo "Please ensure kind cluster is running: make cluster-create"
    exit 1
fi

# Apply fixtures in order
echo ""
echo "Creating namespaces..."
kubectl apply -f "$FIXTURES_DIR/00-namespaces.yaml"

echo ""
echo "Creating risky PVCs and Pods..."
kubectl apply -f "$FIXTURES_DIR/01-risky-pvcs-and-pods.yaml"

echo ""
echo "Creating safe PVCs and Pods..."
kubectl apply -f "$FIXTURES_DIR/02-safe-pvcs-and-pods.yaml"

echo ""
echo "Creating production workload..."
kubectl apply -f "$FIXTURES_DIR/03-production-workload.yaml"

echo ""
echo "Creating staging workload..."
kubectl apply -f "$FIXTURES_DIR/04-staging-workload.yaml"

echo ""
echo "Waiting for PVCs to be bound..."
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc --all -n test-risky --timeout=120s || true
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc --all -n test-safe --timeout=120s || true
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc --all -n test-production --timeout=120s || true
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc --all -n test-staging --timeout=120s || true

echo ""
echo "Waiting for Pods to be ready (this may take a minute)..."
kubectl wait --for=condition=Ready pod --all -n test-risky --timeout=180s || true
kubectl wait --for=condition=Ready pod --all -n test-safe --timeout=180s || true
kubectl wait --for=condition=Ready pod --all -n test-production --timeout=180s || true
kubectl wait --for=condition=Ready pod --all -n test-staging --timeout=180s || true

echo ""
echo "Test fixtures applied successfully!"
echo ""
echo "Summary:"
echo "--------"
echo ""

echo "Namespaces:"
kubectl get namespaces -l pv-safe-test=true

echo ""
echo "PVCs by namespace:"
for ns in test-risky test-safe test-production test-staging; do
    echo ""
    echo "Namespace: $ns"
    kubectl get pvc -n "$ns" -o wide 2>/dev/null || echo "  No PVCs found"
done

echo ""
echo "PVs (all):"
kubectl get pv -o wide

echo ""
echo "Pods by namespace:"
for ns in test-risky test-safe test-production test-staging; do
    echo ""
    echo "Namespace: $ns"
    kubectl get pods -n "$ns" -o wide 2>/dev/null || echo "  No Pods found"
done

echo ""
echo "To view details:"
echo "  kubectl get pvc -n test-risky"
echo "  kubectl get pv"
echo "  kubectl describe pv <pv-name>"
echo ""
