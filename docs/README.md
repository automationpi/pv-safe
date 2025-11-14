# pv-safe Documentation

This directory contains comprehensive documentation for pv-safe, a Kubernetes admission webhook that prevents accidental data loss.

## Documentation Index

### Operator Documentation

- **[OPERATOR_GUIDE.md](OPERATOR_GUIDE.md)** - Complete operator guide including:
  - Installation and setup
  - How the webhook works
  - Common scenarios and resolutions
  - Bypass mechanisms
  - VolumeSnapshot support
  - Monitoring and troubleshooting
  - Best practices

### Quick References

#### Common Commands

**View webhook logs:**
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f
```

**Check webhook status:**
```bash
kubectl get pods -n pv-safe-system
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook
```

**Force delete with bypass (use cautiously):**
```bash
# PVC
kubectl label pvc <name> -n <namespace> pv-safe.io/force-delete=true
kubectl delete pvc <name> -n <namespace>

# Namespace
kubectl label namespace <name> pv-safe.io/force-delete=true
kubectl delete namespace <name>
```

**Create VolumeSnapshot:**
```bash
kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: <pvc-name>-backup
  namespace: <namespace>
spec:
  volumeSnapshotClassName: <snapshot-class>
  source:
    persistentVolumeClaimName: <pvc-name>
EOF
```

#### Risk Assessment Quick Reference

| Resource Type | PV Reclaim Policy | VolumeSnapshot Status | Result |
|--------------|-------------------|----------------------|---------|
| PVC | Retain | N/A | ALLOW |
| PVC | Delete | Ready snapshot (Retain) | ALLOW |
| PVC | Delete | No snapshot | BLOCK |
| PV | Retain | N/A | ALLOW |
| PV | Delete | N/A | BLOCK |
| Namespace | All PVCs safe | N/A | ALLOW |
| Namespace | Any PVC risky | N/A | BLOCK |

#### Common Error Messages

**"PV has Delete reclaim policy, no snapshot found"**
- Cause: PVC bound to PV with Delete policy, no backup exists
- Fix: Create VolumeSnapshot OR change reclaim policy to Retain OR use force-delete label

**"DELETION BLOCKED: Namespace contains N PVC(s) that would lose data permanently"**
- Cause: Namespace has PVCs without snapshots
- Fix: Create VolumeSnapshots for all risky PVCs OR change their reclaim policies

**"admission webhook denied the request"**
- Cause: Risk assessment determined deletion is unsafe
- Fix: Follow suggestions in error message

## Getting Started

1. **New to pv-safe?** Start with [OPERATOR_GUIDE.md](OPERATOR_GUIDE.md) - Overview section
2. **Installing pv-safe?** See [OPERATOR_GUIDE.md](OPERATOR_GUIDE.md) - Installation section
3. **Encountering issues?** Check [OPERATOR_GUIDE.md](OPERATOR_GUIDE.md) - Troubleshooting section
4. **Need to force delete?** See [OPERATOR_GUIDE.md](OPERATOR_GUIDE.md) - Bypass Mechanisms section

## Architecture Overview

```
┌─────────────────────┐
│  kubectl delete     │
│  pvc/ns/pv          │
└──────────┬──────────┘
           │
           v
┌─────────────────────────────────────────────┐
│     Kubernetes API Server                   │
│                                              │
│  ┌────────────────────────────────────┐    │
│  │ ValidatingWebhookConfiguration     │    │
│  │ - Intercepts DELETE operations     │    │
│  │ - Calls pv-safe webhook            │    │
│  └────────────────┬───────────────────┘    │
└───────────────────┼────────────────────────┘
                    │
                    v HTTPS
┌─────────────────────────────────────────────┐
│  pv-safe Webhook (pv-safe-system namespace) │
│                                              │
│  ┌──────────────────────────────────┐       │
│  │  Risk Assessment Engine          │       │
│  │                                   │       │
│  │  1. Check bypass label            │       │
│  │  2. Check PV reclaim policy       │       │
│  │  3. Check VolumeSnapshots         │       │
│  │  4. Return decision (Allow/Deny)  │       │
│  └──────────────────────────────────┘       │
│                                              │
│  Reads (via K8s API):                       │
│  - PersistentVolumes                        │
│  - PersistentVolumeClaims                   │
│  - Namespaces                               │
│  - VolumeSnapshots (if available)           │
│  - VolumeSnapshotClasses (if available)     │
└─────────────────────────────────────────────┘
           │
           v Response (Allow/Deny)
┌─────────────────────┐
│  User sees result:  │
│  - Deletion allowed │
│  - OR detailed      │
│    error message    │
└─────────────────────┘
```

## Key Concepts

### Reclaim Policies

**Retain:**
- PV persists after PVC deletion
- Manual cleanup required
- Safer for production

**Delete:**
- PV automatically deleted with PVC
- Data is permanently lost
- Convenient but risky

### VolumeSnapshots

- Point-in-time copies of PV data
- Created via VolumeSnapshot CRD
- Must be `readyToUse: true`
- VolumeSnapshotClass must have `deletionPolicy: Retain`
- Allows safe deletion of PVCs with Delete policy

### Bypass Label

- Label: `pv-safe.io/force-delete=true`
- Bypasses all safety checks
- Requires explicit action (two-step process)
- Creates audit trail in logs
- Use with extreme caution

## Support

For questions, issues, or contributions:
- Review this documentation
- Check webhook logs: `kubectl logs -n pv-safe-system -l app=pv-safe-webhook`
- Search existing issues
- Open a new issue with details

## Additional Resources

- Project README: `../README.md`
- Implementation Plan: `../IMPLEMENTATION_PLAN.md`
- Test Fixtures: `../test/fixtures/`
- Source Code: `../internal/webhook/`
