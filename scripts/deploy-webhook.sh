#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Deploying pv-safe webhook..."

if ! kubectl get namespace cert-manager &>/dev/null; then
    echo "cert-manager not found. Installing cert-manager..."
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml
    echo "Waiting for cert-manager to be ready..."
    kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
else
    echo "cert-manager is already installed"
fi

echo ""
echo "Applying webhook manifests..."
kubectl apply -f "$PROJECT_ROOT/deploy/00-namespace.yaml"
kubectl apply -f "$PROJECT_ROOT/deploy/01-certificate.yaml"

echo ""
echo "Waiting for certificate to be ready..."
sleep 5
kubectl wait --for=condition=Ready certificate/pv-safe-webhook-cert -n pv-safe-system --timeout=60s || true

echo ""
echo "Applying RBAC..."
kubectl apply -f "$PROJECT_ROOT/deploy/04-rbac.yaml"

echo ""
echo "Deploying webhook..."
kubectl apply -f "$PROJECT_ROOT/deploy/02-deployment.yaml"

echo ""
echo "Waiting for webhook to be ready..."
kubectl wait --for=condition=Available deployment/pv-safe-webhook -n pv-safe-system --timeout=120s

echo ""
echo "Applying webhook configuration..."
kubectl apply -f "$PROJECT_ROOT/deploy/03-webhook-config.yaml"

echo ""
echo "Webhook deployed successfully!"
echo ""
echo "Check status:"
echo "  kubectl get pods -n pv-safe-system"
echo "  kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f"
echo ""
echo "Test deletion:"
echo "  kubectl delete namespace test-development --dry-run=server"
echo "  kubectl delete pvc unused-data -n test-risky --dry-run=server"
