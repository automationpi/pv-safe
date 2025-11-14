#!/bin/bash

set -euo pipefail

CLUSTER_NAME="pv-safe-test"

echo "Deleting kind cluster: $CLUSTER_NAME"

if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster $CLUSTER_NAME does not exist"
    exit 0
fi

kind delete cluster --name "$CLUSTER_NAME"

echo "Cluster $CLUSTER_NAME deleted successfully"
