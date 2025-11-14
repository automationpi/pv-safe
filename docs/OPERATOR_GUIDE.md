# pv-safe Operator Guide

## Table of Contents

1. [Overview](#overview)
2. [Installation](#installation)
3. [How It Works](#how-it-works)
4. [Common Scenarios](#common-scenarios)
5. [Bypass Mechanisms](#bypass-mechanisms)
6. [VolumeSnapshot Support](#volumesnapshot-support)
7. [Monitoring and Observability](#monitoring-and-observability)
8. [Troubleshooting](#troubleshooting)
9. [Best Practices](#best-practices)

## Overview

pv-safe is a Kubernetes admission webhook that prevents accidental data loss by intercepting DELETE operations on:
- Namespaces
- PersistentVolumeClaims (PVCs)
- PersistentVolumes (PVs)

The webhook performs risk assessment based on:
- PersistentVolume reclaim policies (Delete vs Retain)
- Existence of VolumeSnapshots with Retain deletion policy
- Resource binding status

**Key Features:**
- Automatic risk assessment for deletions
- VolumeSnapshot awareness
- Label-based bypass for intentional deletions
- Detailed error messages with remediation steps
- Graceful degradation (works without VolumeSnapshot CRDs)

## Installation

### Prerequisites

- Kubernetes cluster version 1.19+
- cert-manager installed (for TLS certificate management)
- kubectl configured with cluster admin access

### Installation Steps

1. **Clone the repository:**
```bash
git clone <repository-url>
cd pv-safe
```

2. **Build and deploy the webhook:**
```bash
make webhook-build
make webhook-deploy
```

3. **Verify installation:**
```bash
kubectl get pods -n pv-safe-system
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook
```

Expected output:
```
NAME                              READY   STATUS    RESTARTS   AGE
pv-safe-webhook-xxxxxxxxxx-xxxxx   1/1     Running   0          1m
```

### Uninstallation

```bash
make webhook-delete
```

## How It Works

### Admission Flow

1. User attempts to delete a Namespace, PVC, or PV
2. Kubernetes API server sends admission request to pv-safe webhook
3. Webhook performs risk assessment:
   - Checks for bypass label `pv-safe.io/force-delete=true`
   - Evaluates PV reclaim policy
   - Checks for ready VolumeSnapshots with Retain policy
4. Webhook returns decision:
   - **Allow:** Safe deletion or bypass label present
   - **Deny:** Risky deletion with detailed error message

### Risk Assessment Logic

#### For PVC Deletions:

```
Is PVC bound to a PV?
├─ No → ALLOW (nothing to lose)
└─ Yes
   └─ Check PV reclaim policy
      ├─ Retain → ALLOW (PV will persist)
      └─ Delete → Check for VolumeSnapshot
         ├─ Ready snapshot with Retain policy exists → ALLOW
         └─ No snapshot → BLOCK (data loss risk)
```

#### For PV Deletions:

```
Check PV reclaim policy
├─ Retain → ALLOW (manual deletion is intentional)
└─ Delete → BLOCK (data loss risk)
```

#### For Namespace Deletions:

```
List all PVCs in namespace
├─ No PVCs → ALLOW
└─ Has PVCs → Assess each bound PVC
   ├─ All PVCs safe (Retain or have snapshots) → ALLOW
   └─ Any PVC risky → BLOCK (list risky PVCs)
```

### RBAC Permissions

The webhook requires read-only permissions:
- persistentvolumes (get, list)
- persistentvolumeclaims (get, list)
- namespaces (get, list)
- volumesnapshots (get, list)
- volumesnapshotclasses (get, list)

## Common Scenarios

### Scenario 1: Deleting a PVC with Delete Reclaim Policy

**Situation:** You need to delete a PVC, but it has a Delete reclaim policy.

**What happens:**
```bash
$ kubectl delete pvc my-data -n production

Error from server (Forbidden): admission webhook "validate.pv-safe.io" denied the request:
DELETION BLOCKED: PVC 'production/my-data' would lose data permanently

Reason: PV has Delete reclaim policy, no snapshot found

To safely delete this PVC:
  1. Create a VolumeSnapshot of the data
  2. OR change PV reclaim policy to Retain:
     kubectl patch pv pvc-xxx -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
  3. OR force delete (will lose data):
     kubectl label pvc my-data -n production pv-safe.io/force-delete=true
     kubectl delete pvc my-data -n production
  4. Then retry the deletion
```

**Resolution options:**

**Option A: Create a snapshot (recommended)**
```bash
# Create VolumeSnapshot
kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: my-data-backup
  namespace: production
spec:
  volumeSnapshotClassName: <your-snapshot-class>
  source:
    persistentVolumeClaimName: my-data
EOF

# Wait for snapshot to be ready
kubectl wait --for=jsonpath='{.status.readyToUse}'=true \
  volumesnapshot/my-data-backup -n production

# Now deletion is allowed
kubectl delete pvc my-data -n production
```

**Option B: Change reclaim policy**
```bash
# Get PV name
PV_NAME=$(kubectl get pvc my-data -n production -o jsonpath='{.spec.volumeName}')

# Change to Retain
kubectl patch pv $PV_NAME -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'

# Now deletion is allowed
kubectl delete pvc my-data -n production
```

### Scenario 2: Deleting a Namespace with Multiple PVCs

**Situation:** Deleting a namespace that contains PVCs with risky configurations.

**What happens:**
```bash
$ kubectl delete namespace staging

Error from server (Forbidden): admission webhook "validate.pv-safe.io" denied the request:
DELETION BLOCKED: Namespace 'staging' contains 3 PVC(s) that would lose data permanently

Risky PVCs:
  - postgres-data: PV has Delete reclaim policy, no snapshot found
  - redis-data: PV has Delete reclaim policy, no snapshot found
  - elasticsearch-data: PV has Delete reclaim policy, no snapshot found
```

**Resolution:**
```bash
# Create snapshots for all risky PVCs
for pvc in postgres-data redis-data elasticsearch-data; do
  kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: ${pvc}-backup
  namespace: staging
spec:
  volumeSnapshotClassName: <your-snapshot-class>
  source:
    persistentVolumeClaimName: $pvc
EOF
done

# Wait for all snapshots to be ready
kubectl wait --for=jsonpath='{.status.readyToUse}'=true \
  volumesnapshot --all -n staging

# Now namespace deletion is allowed
kubectl delete namespace staging
```

### Scenario 3: Emergency Deletion Without Backup

**Situation:** You need to immediately delete a resource and understand the data loss risk.

**Resolution:**
```bash
# For PVC
kubectl label pvc my-data -n production pv-safe.io/force-delete=true
kubectl delete pvc my-data -n production

# For namespace
kubectl label namespace staging pv-safe.io/force-delete=true
kubectl delete namespace staging

# For PV
kubectl label pv pvc-xxx pv-safe.io/force-delete=true
kubectl delete pv pvc-xxx
```

**Important:** This bypasses all safety checks. Data will be lost permanently.

## Bypass Mechanisms

### Label-Based Bypass

To force delete a resource, add the label `pv-safe.io/force-delete=true`.

**Why two steps?**
- Forces explicit acknowledgment of data loss risk
- Creates audit trail (label + deletion both logged)
- Prevents accidental deletions

**Audit trail:**
The webhook logs all bypass operations:
```
BYPASS: Force delete label found on PersistentVolumeClaim test-risky/my-data
  User: kubernetes-admin
  Allowing deletion despite potential data loss
```

### Namespace Exclusions

The webhook is configured to exclude certain namespaces from validation:
- kube-system
- kube-public
- kube-node-lease
- pv-safe-system

See `deploy/05-webhook-config.yaml` to modify exclusions.

## VolumeSnapshot Support

### Overview

The webhook integrates with Kubernetes VolumeSnapshot API to allow safe deletions when backups exist.

### Requirements

- VolumeSnapshot CRDs installed (v1)
- CSI driver with snapshot support
- VolumeSnapshotClass with `deletionPolicy: Retain`

### How Snapshot Detection Works

When assessing PVC deletion risk:
1. Check if PV has Delete reclaim policy
2. Query for VolumeSnapshots in the PVC's namespace
3. Find snapshots where `spec.source.persistentVolumeClaimName` matches PVC
4. Check if snapshot is `readyToUse: true`
5. Verify VolumeSnapshotClass has `deletionPolicy: Retain`

If all conditions met → Deletion allowed

### Installing Snapshot Support

```bash
# Install VolumeSnapshot CRDs
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v6.3.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v6.3.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v6.3.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml

# Install snapshot controller
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v6.3.0/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v6.3.0/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```

### Creating a VolumeSnapshotClass

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-snapclass
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: <your-csi-driver>  # e.g., ebs.csi.aws.com
deletionPolicy: Retain
```

### Graceful Degradation

If VolumeSnapshot CRDs are not installed:
- Webhook logs warning: "Failed to create Snapshot checker"
- Webhook continues to function
- Only reclaim policy checks are performed
- No snapshot-based allowances

## Monitoring and Observability

### Webhook Logs

View webhook logs:
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f
```

**Log levels:**
- INFO: Normal operations (all requests logged)
- WARNING: Snapshot API unavailable
- ERROR: Internal errors (fail-open behavior)

**Key log messages:**
```
# Deletion blocked
BLOCKING: Risky deletion detected!
  Reason: PV has Delete reclaim policy, no snapshot found

# Deletion allowed with snapshot
ALLOWING: Deletion is safe
  Reason: Ready VolumeSnapshot 'my-backup' exists with Retain policy

# Bypass used
BYPASS: Force delete label found on PersistentVolumeClaim production/data
  User: alice@company.com
  Allowing deletion despite potential data loss
```

### Health Checks

**Liveness probe:**
```bash
kubectl exec -n pv-safe-system <pod-name> -- curl http://localhost:8443/healthz
```

**Readiness probe:**
```bash
kubectl exec -n pv-safe-system <pod-name> -- curl http://localhost:8443/readyz
```

### Metrics (Future Enhancement)

Planned metrics:
- `pvsafe_deletions_blocked_total` - Counter of blocked deletions
- `pvsafe_deletions_allowed_total` - Counter of allowed deletions
- `pvsafe_bypass_used_total` - Counter of bypass label usage
- `pvsafe_snapshot_checks_total` - Counter of snapshot checks

## Troubleshooting

### Issue: Webhook Blocks All Deletions

**Symptoms:**
- All PVC/PV deletions blocked, even with Retain policy
- Error message: "Risk assessment error (allowed)"

**Causes:**
1. Webhook cannot connect to Kubernetes API
2. RBAC permissions missing

**Resolution:**
```bash
# Check webhook pod status
kubectl get pods -n pv-safe-system

# Check RBAC
kubectl auth can-i get pv --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
kubectl auth can-i list pvc --as=system:serviceaccount:pv-safe-system:pv-safe-webhook

# Check webhook logs for errors
kubectl logs -n pv-safe-system -l app=pv-safe-webhook
```

### Issue: Webhook Not Called

**Symptoms:**
- Deletions proceed without webhook validation
- No entries in webhook logs

**Causes:**
1. ValidatingWebhookConfiguration not created
2. Webhook service unreachable
3. Certificate issues

**Resolution:**
```bash
# Check webhook configuration exists
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook

# Check webhook service
kubectl get svc -n pv-safe-system pv-safe-webhook

# Check certificate
kubectl get certificate -n pv-safe-system pv-safe-webhook-cert

# Test webhook connectivity
kubectl run test-pod --rm -i --restart=Never --image=curlimages/curl -- \
  curl -k https://pv-safe-webhook.pv-safe-system.svc:443/healthz
```

### Issue: Snapshot Not Detected

**Symptoms:**
- VolumeSnapshot exists and is ready
- Webhook still blocks deletion
- Message: "no snapshot found"

**Causes:**
1. Snapshot not in same namespace as PVC
2. Snapshot not `readyToUse: true`
3. VolumeSnapshotClass has `deletionPolicy: Delete`
4. RBAC missing for volumesnapshots

**Resolution:**
```bash
# Check snapshot status
kubectl get volumesnapshot -n <namespace>
kubectl get volumesnapshot <name> -n <namespace> -o yaml

# Check VolumeSnapshotClass
kubectl get volumesnapshotclass <class-name> -o yaml | grep deletionPolicy

# Check webhook RBAC for snapshots
kubectl auth can-i get volumesnapshots --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
kubectl auth can-i get volumesnapshotclasses --as=system:serviceaccount:pv-safe-system:pv-safe-webhook

# Check webhook logs for snapshot checker initialization
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep -i snapshot
```

### Issue: Webhook Timeout

**Symptoms:**
- DELETE operations hang
- Error: "context deadline exceeded"

**Causes:**
1. Webhook taking too long (>10s timeout)
2. Large namespace with many PVCs

**Resolution:**
```bash
# Check webhook latency in logs
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep -i timeout

# Increase webhook timeout in configuration
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/timeoutSeconds", "value": 30}]'
```

### Issue: PV Deletion Blocked After PVC Deleted

**Symptoms:**
- PVC was deleted (had Retain policy)
- PV deletion now blocked

**Causes:**
- PV has Delete reclaim policy (unusual but possible)
- No snapshot reference available (PVC is gone)

**Resolution:**
```bash
# Option 1: Change PV reclaim policy
kubectl patch pv <pv-name> -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'

# Option 2: Use bypass
kubectl label pv <pv-name> pv-safe.io/force-delete=true
kubectl delete pv <pv-name>
```

## Best Practices

### 1. Use VolumeSnapshots for Production Data

Configure regular snapshots for critical data:
```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: daily-backup-20250113
  namespace: production
spec:
  volumeSnapshotClassName: production-snapclass
  source:
    persistentVolumeClaimName: database-data
```

### 2. Set Appropriate Reclaim Policies

**For development/testing:**
```yaml
reclaimPolicy: Delete  # Automatic cleanup
```

**For production:**
```yaml
reclaimPolicy: Retain  # Manual cleanup, safer
```

### 3. Document Force Deletions

When using bypass label:
```bash
# Create audit log entry
echo "$(date): Force deleting PVC production/my-data - Reason: Corrupted data, no recovery needed" >> deletion-audit.log

kubectl label pvc my-data -n production pv-safe.io/force-delete=true
kubectl delete pvc my-data -n production
```

### 4. Test Webhook in Non-Production First

```bash
# Deploy to staging cluster first
kubectl config use-context staging
make webhook-deploy

# Test various scenarios
kubectl delete pvc test-pvc -n test-namespace
```

### 5. Monitor Webhook Health

Set up alerts for:
- Webhook pod restarts
- High error rates in logs
- Webhook timeout errors
- Certificate expiration (cert-manager handles renewal)

### 6. Regular Backup Strategy

Even with pv-safe:
- Maintain regular backup schedule
- Test restore procedures
- Document recovery processes
- Keep snapshots with Retain policy

### 7. Namespace Lifecycle Management

For temporary namespaces:
```bash
# Option 1: Use Retain policy
kubectl patch pv <pv-name> -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'

# Option 2: Create snapshots before deletion
for pvc in $(kubectl get pvc -n temp-namespace -o name); do
  # Create snapshot
  # Wait for ready
done

# Then delete namespace
kubectl delete namespace temp-namespace
```

### 8. Educate Development Teams

- Share this operator guide
- Explain risk assessment logic
- Document bypass procedures
- Provide snapshot creation examples

## Advanced Configuration

### Webhook Failure Policy

Current setting: `Fail` (blocks deletions if webhook unavailable)

To change to fail-open (allow deletions if webhook down):
```bash
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/failurePolicy", "value": "Ignore"}]'
```

**Warning:** Fail-open reduces safety guarantees.

### Exclude Additional Namespaces

Edit `deploy/05-webhook-config.yaml`:
```yaml
namespaceSelector:
  matchExpressions:
  - key: name
    operator: NotIn
    values:
    - kube-system
    - kube-public
    - kube-node-lease
    - pv-safe-system
    - your-namespace-here
```

Then reapply:
```bash
kubectl apply -f deploy/05-webhook-config.yaml
```

### Custom Bypass Label

To use a different label, modify `internal/webhook/handler.go`:
```go
const (
    BypassLabel = "your-company.io/force-delete"
)
```

Then rebuild and redeploy.

## Support and Contributing

**Issues:** https://github.com/automationpi/pv-safe/issues
**Documentation:** https://github.com/automationpi/pv-safe/docs

For questions or issues, please:
1. Check this operator guide
2. Review webhook logs
3. Search existing issues
4. Open a new issue with logs and details
