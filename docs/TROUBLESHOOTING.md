# Troubleshooting Guide

Common issues and solutions when using pv-safe.

## Table of Contents

- [Webhook Not Responding](#webhook-not-responding)
- [Deletions Not Being Blocked](#deletions-not-being-blocked)
- [All Deletions Blocked](#all-deletions-blocked)
- [Snapshot Not Detected](#snapshot-not-detected)
- [Certificate Issues](#certificate-issues)
- [Performance Problems](#performance-problems)
- [Bypass Label Not Working](#bypass-label-not-working)

## Webhook Not Responding

### Symptoms

```
Error from server (InternalError): Internal error occurred:
failed calling webhook "validate.pv-safe.io":
Post "https://pv-safe-webhook.pv-safe-system.svc:443/validate":
dial tcp x.x.x.x:443: i/o timeout
```

### Diagnosis

```bash
# Check if webhook pods are running
kubectl get pods -n pv-safe-system

# Check pod logs for errors
kubectl logs -n pv-safe-system -l app=pv-safe-webhook

# Check service endpoints
kubectl get endpoints -n pv-safe-system pv-safe-webhook

# Test webhook endpoint directly
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -k https://pv-safe-webhook.pv-safe-system.svc:443/healthz
```

### Solutions

**1. Webhook pods not running:**
```bash
# Check deployment status
kubectl describe deployment pv-safe-webhook -n pv-safe-system

# Check for image pull errors
kubectl describe pods -n pv-safe-system -l app=pv-safe-webhook

# Restart deployment
kubectl rollout restart deployment pv-safe-webhook -n pv-safe-system
```

**2. Service not routing to pods:**
```bash
# Check service configuration
kubectl get service pv-safe-webhook -n pv-safe-system -o yaml

# Verify selector matches pod labels
kubectl get pods -n pv-safe-system --show-labels
```

**3. Certificate issues:**

See [Certificate Issues](#certificate-issues) section below.

## Deletions Not Being Blocked

### Symptoms

PVCs with Delete reclaim policy and no snapshots are being deleted without errors.

### Diagnosis

```bash
# Check if webhook is registered
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook

# Check webhook rules
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook -o yaml

# Check webhook logs
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --tail=100

# Test a deletion and watch logs in real-time
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f &
kubectl delete pvc <test-pvc> -n <namespace>
```

### Solutions

**1. Webhook not receiving requests:**

Check webhook configuration matches resources:
```bash
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook -o jsonpath='{.webhooks[0].rules}'
```

Expected output should include:
```json
[
  {
    "apiGroups": [""],
    "apiVersions": ["v1"],
    "operations": ["DELETE"],
    "resources": ["persistentvolumeclaims", "namespaces"],
    "scope": "*"
  }
]
```

**2. Namespace excluded from webhook:**

Check if namespace has exclusion label:
```bash
kubectl get namespace <namespace> --show-labels
```

Webhook may exclude certain namespaces (like `kube-system`). Check webhook configuration:
```bash
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook \
  -o jsonpath='{.webhooks[0].namespaceSelector}'
```

**3. Webhook failing open:**

Check failure policy:
```bash
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook \
  -o jsonpath='{.webhooks[0].failurePolicy}'
```

Should be `Fail` (fail-closed). If it's `Ignore`, webhook failures won't block deletions.

**4. RBAC permissions missing:**

Verify webhook has permissions to read PVCs and PVs:
```bash
kubectl auth can-i get persistentvolumeclaims \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook \
  --all-namespaces

kubectl auth can-i list persistentvolumes \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
```

Both should return `yes`.

## All Deletions Blocked

### Symptoms

Even PVCs with Retain reclaim policy or valid snapshots are being blocked.

### Diagnosis

```bash
# Check webhook logs for errors
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep ERROR

# Describe the PVC
kubectl describe pvc <pvc-name> -n <namespace>

# Check PV reclaim policy
kubectl get pv <pv-name> -o jsonpath='{.spec.persistentVolumeReclaimPolicy}'

# If should have snapshot, check snapshot status
kubectl get volumesnapshot -n <namespace>
kubectl describe volumesnapshot <snapshot-name> -n <namespace>
```

### Solutions

**1. PV not found or not bound:**

Webhook allows deletion if no PV is bound. Check PVC status:
```bash
kubectl get pvc <pvc-name> -n <namespace> -o jsonpath='{.status.phase}'
```

If status is not `Bound`, the PVC has no data to lose.

**2. Snapshot not being detected:**

See [Snapshot Not Detected](#snapshot-not-detected) section below.

**3. API server connection issues:**

Check webhook logs for API errors:
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep "failed to get"
```

Verify RBAC permissions (see previous section).

**4. Timeout issues:**

If webhook times out, it may block by default (fail-closed):
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep timeout
```

Check webhook timeout configuration:
```bash
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook \
  -o jsonpath='{.webhooks[0].timeoutSeconds}'
```

Increase timeout if needed:
```bash
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/timeoutSeconds", "value": 30}]'
```

## Snapshot Not Detected

### Symptoms

VolumeSnapshot exists and is ready, but webhook still blocks deletion.

### Diagnosis

```bash
# Check snapshot details
kubectl get volumesnapshot <snapshot-name> -n <namespace> -o yaml

# Verify snapshot is ready
kubectl get volumesnapshot <snapshot-name> -n <namespace> \
  -o jsonpath='{.status.readyToUse}'
# Should return: true

# Check snapshot references correct PVC
kubectl get volumesnapshot <snapshot-name> -n <namespace> \
  -o jsonpath='{.spec.source.persistentVolumeClaimName}'

# Check VolumeSnapshotClass deletion policy
SNAPSHOT_CLASS=$(kubectl get volumesnapshot <snapshot-name> -n <namespace> \
  -o jsonpath='{.spec.volumeSnapshotClassName}')
kubectl get volumesnapshotclass $SNAPSHOT_CLASS \
  -o jsonpath='{.deletionPolicy}'
# Should return: Retain
```

### Solutions

**1. Snapshot not ready:**

Wait for snapshot to become ready:
```bash
kubectl wait --for=jsonpath='{.status.readyToUse}'=true \
  volumesnapshot/<snapshot-name> -n <namespace> --timeout=300s
```

**2. Snapshot has Delete policy:**

Webhook only considers snapshots with Retain deletion policy as safe backups.

Check VolumeSnapshotClass:
```bash
kubectl get volumesnapshotclass <class-name> -o yaml
```

Change deletion policy to Retain:
```bash
kubectl patch volumesnapshotclass <class-name> \
  --type='json' -p='[{"op": "replace", "path": "/deletionPolicy", "value": "Retain"}]'
```

**3. VolumeSnapshot CRDs not installed:**

Check if CRDs exist:
```bash
kubectl get crd | grep snapshot
```

Should show:
```
volumesnapshotclasses.snapshot.storage.k8s.io
volumesnapshotcontents.snapshot.storage.k8s.io
volumesnapshots.snapshot.storage.k8s.io
```

If missing, install VolumeSnapshot CRDs:
```bash
VERSION=v6.3.0
BASE_URL=https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${VERSION}

kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
```

**4. Webhook missing RBAC for snapshots:**

Verify permissions:
```bash
kubectl auth can-i get volumesnapshots \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook \
  -n <namespace>

kubectl auth can-i get volumesnapshotclasses \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
```

Both should return `yes`.

**5. Snapshot in different namespace:**

Webhook only checks snapshots in the same namespace as the PVC. Cross-namespace snapshots are not supported.

## Certificate Issues

### Symptoms

```
Error from server (InternalError): Internal error occurred:
failed calling webhook "validate.pv-safe.io":
x509: certificate signed by unknown authority
```

### Diagnosis

```bash
# Check cert-manager is running
kubectl get pods -n cert-manager

# Check certificate status
kubectl get certificate -n pv-safe-system

# Check certificate details
kubectl describe certificate pv-safe-webhook-cert -n pv-safe-system

# Check if secret exists
kubectl get secret pv-safe-webhook-tls -n pv-safe-system

# Check webhook caBundle
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d
```

### Solutions

**1. cert-manager not installed:**

Install cert-manager:
```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=available --timeout=300s \
  deployment/cert-manager -n cert-manager
```

**2. Certificate not ready:**

Check certificate status:
```bash
kubectl describe certificate pv-safe-webhook-cert -n pv-safe-system
```

If certificate is not ready, check cert-manager logs:
```bash
kubectl logs -n cert-manager -l app=cert-manager
```

**3. Certificate expired:**

Delete certificate to trigger renewal:
```bash
kubectl delete certificate pv-safe-webhook-cert -n pv-safe-system
kubectl delete secret pv-safe-webhook-tls -n pv-safe-system
```

cert-manager will auto-recreate.

**4. Webhook caBundle not updated:**

The caBundle in ValidatingWebhookConfiguration should match the CA certificate. With cert-manager's `cert-manager.io/inject-ca-from` annotation, this should be automatic.

Verify annotation exists:
```bash
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook \
  -o jsonpath='{.metadata.annotations}'
```

Should include: `cert-manager.io/inject-ca-from: pv-safe-system/pv-safe-webhook-cert`

## Performance Problems

### Symptoms

- Slow DELETE operations
- Webhook timeout errors
- High latency in logs

### Diagnosis

```bash
# Check webhook resource usage
kubectl top pods -n pv-safe-system

# Check for resource limits
kubectl describe deployment pv-safe-webhook -n pv-safe-system | grep -A 5 Limits

# Check webhook logs for slow operations
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep -i "took\|duration\|slow"

# Check API server performance
kubectl get --raw /metrics | grep apiserver_request_duration
```

### Solutions

**1. Increase resource limits:**

```bash
kubectl patch deployment pv-safe-webhook -n pv-safe-system \
  --type='json' -p='[
    {"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/cpu", "value": "500m"},
    {"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value": "256Mi"}
  ]'
```

**2. Increase replicas:**

```bash
kubectl scale deployment pv-safe-webhook -n pv-safe-system --replicas=3
```

**3. Increase webhook timeout:**

```bash
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/timeoutSeconds", "value": 30}]'
```

**4. Check API server health:**

Webhook makes API calls to get PVCs, PVs, and snapshots. If API server is slow, webhook will be slow.

```bash
# Check API server metrics
kubectl top nodes
kubectl get --raw /metrics | grep apiserver
```

## Bypass Label Not Working

### Symptoms

PVC has `pv-safe.io/force-delete=true` label but deletion is still blocked.

### Diagnosis

```bash
# Verify label is set correctly
kubectl get pvc <pvc-name> -n <namespace> --show-labels

# Check webhook logs
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep -i bypass
```

### Solutions

**1. Label value incorrect:**

Label must be exactly `pv-safe.io/force-delete=true` (not `"true"`, not `yes`).

Correct way:
```bash
kubectl label pvc <pvc-name> -n <namespace> pv-safe.io/force-delete=true
```

**2. Label on wrong resource:**

For namespace deletion, label the namespace:
```bash
kubectl label namespace <namespace> pv-safe.io/force-delete=true
```

For PVC deletion, label the PVC:
```bash
kubectl label pvc <pvc-name> -n <namespace> pv-safe.io/force-delete=true
```

**3. Webhook not detecting label:**

Check webhook logs to see if bypass is detected:
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --tail=50 | grep BYPASS
```

If no BYPASS log, webhook may not be parsing labels correctly. Check webhook version.

## Getting More Help

### Enable Debug Logging

If standard troubleshooting doesn't help, enable verbose logging:

```bash
# Edit deployment to add debug environment variable
kubectl set env deployment/pv-safe-webhook -n pv-safe-system DEBUG=true

# Watch logs with more detail
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f
```

### Collect Diagnostic Information

When reporting issues, collect this information:

```bash
# Webhook status
kubectl get pods -n pv-safe-system
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --tail=200

# Configuration
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook -o yaml
kubectl get certificate -n pv-safe-system
kubectl get deployment pv-safe-webhook -n pv-safe-system -o yaml

# Resource being deleted
kubectl describe pvc <pvc-name> -n <namespace>
kubectl get pv <pv-name> -o yaml

# If snapshots involved
kubectl get volumesnapshot -n <namespace>
kubectl describe volumesnapshot <snapshot-name> -n <namespace>

# Kubernetes version
kubectl version
```

### Report Issues

If you've tried the above and still have issues:

1. **Search existing issues:** [GitHub Issues](https://github.com/automationpi/pv-safe/issues)
2. **Create new issue:** Include diagnostic information above
3. **Ask in discussions:** [GitHub Discussions](https://github.com/automationpi/pv-safe/discussions)

### Emergency: Disable Webhook

If webhook is blocking critical operations and you need to disable it immediately:

```bash
# Delete webhook configuration (operations will no longer be validated)
kubectl delete validatingwebhookconfiguration pv-safe-validating-webhook

# Or change to fail-open temporarily
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/failurePolicy", "value": "Ignore"}]'
```

**Warning:** This removes all protection. Only use in emergencies.

Re-enable after issue is resolved:
```bash
# Redeploy webhook
helm upgrade pv-safe oci://ghcr.io/automationpi/pv-safe

# Or re-apply manifests
kubectl apply -f deploy/
```
