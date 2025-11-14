#!/bin/bash

set -euo pipefail

CLUSTER_NAME="pv-safe-test"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Creating kind cluster: $CLUSTER_NAME"

# Check if cluster already exists
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster $CLUSTER_NAME already exists"
    read -p "Do you want to delete and recreate it? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Deleting existing cluster..."
        kind delete cluster --name "$CLUSTER_NAME"
        # Clean up Docker resources
        echo "Cleaning up Docker resources..."
        docker system prune -f > /dev/null 2>&1 || true
        sleep 2
    else
        echo "Exiting without changes"
        exit 0
    fi
fi

# Create cluster with config file
echo "Creating kind cluster with config..."
echo "This may take 2-5 minutes..."
set +e  # Temporarily disable exit on error
kind create cluster --config "$PROJECT_ROOT/kind-config.yaml" --wait 10m
CLUSTER_CREATE_EXIT_CODE=$?
set -e  # Re-enable exit on error

if [ $CLUSTER_CREATE_EXIT_CODE -ne 0 ]; then
    echo ""
    echo "WARNING: Cluster creation with config file failed (multi-node setup)"
    echo "Trying simplified approach without config file (single-node cluster)..."
    echo ""
    # Fallback: create cluster without config
    if ! kind create cluster --name "$CLUSTER_NAME" --wait 10m; then
        echo "ERROR: Both cluster creation attempts failed"
        exit 1
    fi
    echo "Single-node cluster created successfully"
fi

# Verify cluster is ready
echo "Verifying cluster is ready..."
kubectl cluster-info --context "kind-$CLUSTER_NAME"

# Label worker nodes for testing (if they exist)
echo "Labeling worker nodes..."
WORKER_NODES=$(kubectl get nodes --no-headers | grep worker | awk '{print $1}' || true)
if [ -n "$WORKER_NODES" ]; then
    echo "$WORKER_NODES" | while read node; do
        kubectl label node "$node" node-role.kubernetes.io/worker=worker --overwrite || true
    done
    echo "Worker nodes labeled"
else
    echo "No worker nodes found (single-node cluster)"
fi

# Install local-path-provisioner (already included in kind, but ensure it's ready)
echo "Waiting for local-path-provisioner to be ready..."
if ! kubectl wait --for=condition=Ready pod -l app=local-path-provisioner -n local-path-storage --timeout=120s 2>/dev/null; then
    echo "Warning: local-path-provisioner may not be ready yet, but continuing..."
fi

# Create default storage class if not exists
echo "Verifying default storage class..."
if ! kubectl get storageclass standard &>/dev/null; then
    echo "Creating default storage class..."
    cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
EOF
else
    echo "Default storage class 'standard' already exists"
fi

# Create storage class with Retain policy for testing
echo "Creating storage class with Retain policy..."
cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard-retain
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Retain
EOF

echo ""
echo "Cluster $CLUSTER_NAME created successfully!"
echo ""
echo "Cluster info:"
kubectl get nodes --context "kind-$CLUSTER_NAME"
echo ""
echo "Storage classes:"
kubectl get storageclass --context "kind-$CLUSTER_NAME"
echo ""
echo "To use this cluster:"
echo "  kubectl config use-context kind-$CLUSTER_NAME"
echo ""
echo "To load test fixtures:"
echo "  make test-fixtures-apply"
echo ""

exit 0
