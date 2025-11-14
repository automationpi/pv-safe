# pv-safe Quick Reference

## Installation

```bash
# Deploy webhook
make webhook-build
make webhook-deploy

# Verify
kubectl get pods -n pv-safe-system
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook
```

## Common Operations

### View Webhook Logs
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f
```

### Check Webhook Health
```bash
kubectl get pods -n pv-safe-system
kubectl describe pod -n pv-safe-system -l app=pv-safe-webhook
```

### Restart Webhook
```bash
kubectl rollout restart deployment/pv-safe-webhook -n pv-safe-system
```

## Handling Blocked Deletions

### Option 1: Create VolumeSnapshot (Recommended)

```bash
# Create snapshot
kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: <pvc-name>-backup-$(date +%Y%m%d)
  namespace: <namespace>
spec:
  volumeSnapshotClassName: <your-snapshot-class>
  source:
    persistentVolumeClaimName: <pvc-name>
EOF

# Wait for snapshot to be ready
kubectl wait --for=jsonpath='{.status.readyToUse}'=true \
  volumesnapshot/<pvc-name>-backup-$(date +%Y%m%d) -n <namespace> \
  --timeout=300s

# Retry deletion
kubectl delete pvc <pvc-name> -n <namespace>
```

### Option 2: Change Reclaim Policy

```bash
# Get PV name
PV_NAME=$(kubectl get pvc <pvc-name> -n <namespace> -o jsonpath='{.spec.volumeName}')

# Change to Retain
kubectl patch pv $PV_NAME -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'

# Retry deletion
kubectl delete pvc <pvc-name> -n <namespace>
```

### Option 3: Force Delete (Data Loss)

```bash
# For PVC
kubectl label pvc <pvc-name> -n <namespace> pv-safe.io/force-delete=true
kubectl delete pvc <pvc-name> -n <namespace>

# For Namespace
kubectl label namespace <namespace> pv-safe.io/force-delete=true
kubectl delete namespace <namespace>

# For PV
kubectl label pv <pv-name> pv-safe.io/force-delete=true
kubectl delete pv <pv-name>
```

## Decision Matrix

| PV Policy | Snapshot Exists | Snapshot Ready | Snapshot Class Policy | Result |
|-----------|----------------|----------------|----------------------|---------|
| Retain | - | - | - | ALLOW |
| Delete | No | - | - | BLOCK |
| Delete | Yes | No | - | BLOCK |
| Delete | Yes | Yes | Delete | BLOCK |
| Delete | Yes | Yes | Retain | ALLOW |

## Troubleshooting Commands

### Webhook Not Working

```bash
# Check webhook configuration
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook -o yaml

# Check webhook service
kubectl get svc -n pv-safe-system pv-safe-webhook

# Check webhook endpoint
kubectl get endpoints -n pv-safe-system pv-safe-webhook

# Check certificate
kubectl get certificate -n pv-safe-system pv-safe-webhook-cert
kubectl describe certificate -n pv-safe-system pv-safe-webhook-cert
```

### Snapshot Not Detected

```bash
# Check snapshot exists
kubectl get volumesnapshot -n <namespace>

# Check snapshot status
kubectl get volumesnapshot <snapshot-name> -n <namespace> -o yaml

# Check snapshot is ready
kubectl get volumesnapshot <snapshot-name> -n <namespace> \
  -o jsonpath='{.status.readyToUse}'

# Check snapshot class
kubectl get volumesnapshotclass <class-name> -o yaml | grep deletionPolicy

# Check webhook can access snapshots
kubectl auth can-i get volumesnapshots \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
```

### Check RBAC Permissions

```bash
# Check all required permissions
for resource in pv pvc namespaces volumesnapshots volumesnapshotclasses; do
  echo "Checking $resource..."
  kubectl auth can-i get $resource \
    --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
  kubectl auth can-i list $resource \
    --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
done
```

## Bulk Operations

### Create Snapshots for All PVCs in Namespace

```bash
NAMESPACE=<your-namespace>
SNAPSHOT_CLASS=<your-snapshot-class>

for pvc in $(kubectl get pvc -n $NAMESPACE -o jsonpath='{.items[*].metadata.name}'); do
  echo "Creating snapshot for $pvc..."
  kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: ${pvc}-backup-$(date +%Y%m%d-%H%M%S)
  namespace: $NAMESPACE
spec:
  volumeSnapshotClassName: $SNAPSHOT_CLASS
  source:
    persistentVolumeClaimName: $pvc
EOF
done
```

### Change Reclaim Policy for All PVs in Namespace

```bash
NAMESPACE=<your-namespace>

for pv in $(kubectl get pvc -n $NAMESPACE -o jsonpath='{.items[*].spec.volumeName}'); do
  echo "Patching PV $pv to Retain..."
  kubectl patch pv $pv -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
done
```

### List Risky PVCs (Delete Policy, No Snapshot)

```bash
NAMESPACE=<your-namespace>

echo "PVCs with Delete reclaim policy:"
for pvc in $(kubectl get pvc -n $NAMESPACE -o jsonpath='{.items[*].metadata.name}'); do
  PV=$(kubectl get pvc $pvc -n $NAMESPACE -o jsonpath='{.spec.volumeName}')
  POLICY=$(kubectl get pv $PV -o jsonpath='{.spec.persistentVolumeReclaimPolicy}')
  SNAPSHOT_COUNT=$(kubectl get volumesnapshot -n $NAMESPACE \
    --field-selector spec.source.persistentVolumeClaimName=$pvc \
    --no-headers 2>/dev/null | wc -l)

  if [ "$POLICY" = "Delete" ]; then
    echo "  - $pvc (PV: $PV, Snapshots: $SNAPSHOT_COUNT)"
  fi
done
```

## Monitoring

### Watch Webhook Logs for Blocks

```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f | grep BLOCKING
```

### Watch Webhook Logs for Bypass Usage

```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f | grep BYPASS
```

### Count Recent Blocks

```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --since=24h | \
  grep -c "BLOCKING: Risky deletion detected"
```

### List Recent Force Deletions

```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --since=24h | \
  grep "BYPASS:" -A 3
```

## VolumeSnapshot Setup

### Install Snapshot CRDs

```bash
VERSION=v6.3.0
BASE_URL=https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${VERSION}

kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml

kubectl apply -f ${BASE_URL}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f ${BASE_URL}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```

### Create VolumeSnapshotClass

```bash
kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: production-snapclass
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: <your-csi-driver>  # e.g., ebs.csi.aws.com
deletionPolicy: Retain
EOF
```

### Verify Snapshot Support

```bash
# Check snapshot controller
kubectl get pods -n kube-system | grep snapshot-controller

# Check snapshot CRDs
kubectl get crd | grep volumesnapshot

# List snapshot classes
kubectl get volumesnapshotclass
```

## Emergency Procedures

### Disable Webhook Temporarily

```bash
# Delete webhook configuration (deletions will proceed without checks)
kubectl delete validatingwebhookconfiguration pv-safe-validating-webhook

# Re-enable later
kubectl apply -f deploy/05-webhook-config.yaml
```

### Change to Fail-Open Mode

```bash
# Allow deletions if webhook is down
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/failurePolicy", "value": "Ignore"}]'

# Revert to fail-closed (safer)
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/failurePolicy", "value": "Fail"}]'
```

## Useful Aliases

Add to your `.bashrc` or `.zshrc`:

```bash
# pv-safe aliases
alias pvsafe-logs='kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f'
alias pvsafe-status='kubectl get pods -n pv-safe-system'
alias pvsafe-restart='kubectl rollout restart deployment/pv-safe-webhook -n pv-safe-system'

# Force delete helpers (use with caution)
pvsafe-force-pvc() {
  kubectl label pvc $1 -n $2 pv-safe.io/force-delete=true
  kubectl delete pvc $1 -n $2
}

pvsafe-force-ns() {
  kubectl label namespace $1 pv-safe.io/force-delete=true
  kubectl delete namespace $1
}

# Snapshot helpers
pvsafe-snap() {
  local PVC=$1
  local NS=$2
  local CLASS=${3:-csi-snapclass}
  kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: ${PVC}-backup-$(date +%Y%m%d-%H%M%S)
  namespace: $NS
spec:
  volumeSnapshotClassName: $CLASS
  source:
    persistentVolumeClaimName: $PVC
EOF
}
```

## Configuration Files

- Webhook deployment: `deploy/03-deployment.yaml`
- RBAC permissions: `deploy/04-rbac.yaml`
- Webhook configuration: `deploy/05-webhook-config.yaml`
- Source code: `internal/webhook/`

## Support Resources

- Full Operator Guide: `OPERATOR_GUIDE.md`
- Documentation Index: `README.md`
- Project README: `../README.md`
