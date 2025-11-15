# pv-safe

[![CI](https://github.com/automationpi/pv-safe/actions/workflows/ci.yaml/badge.svg)](https://github.com/automationpi/pv-safe/actions/workflows/ci.yaml)
[![Build](https://github.com/automationpi/pv-safe/actions/workflows/build.yaml/badge.svg)](https://github.com/automationpi/pv-safe/actions/workflows/build.yaml)
[![License](https://img.shields.io/github/license/automationpi/pv-safe)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/automationpi/pv-safe)](https://github.com/automationpi/pv-safe/releases)

> A Kubernetes admission webhook that prevents accidental data loss from PersistentVolume deletions

pv-safe acts as a safety gate for Kubernetes storage operations, automatically blocking risky deletion attempts and providing clear guidance for safe data management.

## Features

- **Automatic Risk Assessment** - Analyzes PV reclaim policies and VolumeSnapshot availability before allowing deletions
- **Smart Blocking** - Prevents data loss while allowing safe operations to proceed
- **VolumeSnapshot Aware** - Recognizes when backups exist and permits deletion accordingly
- **Clear Error Messages** - Provides actionable guidance with specific commands to resolve issues
- **Minimal Overhead** - Read-only permissions, no data modification
- **Graceful Degradation** - Works without VolumeSnapshot CRDs installed

## Quick Start

### Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- cert-manager (optional but recommended)

### Install with Helm (Recommended)

```bash
# Install cert-manager if not already installed
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Install pv-safe from OCI registry
helm install pv-safe oci://ghcr.io/automationpi/pv-safe --version 0.1.0

# Or install from source
git clone https://github.com/automationpi/pv-safe.git
cd pv-safe
helm install pv-safe ./charts/pv-safe

# Verify installation
kubectl get pods -n pv-safe-system
```

### Alternative: Install from Source

For development or customization:

```bash
# Clone and build
git clone https://github.com/automationpi/pv-safe.git
cd pv-safe
make webhook-build
make webhook-deploy
```

### Basic Usage

pv-safe automatically intercepts DELETE operations on Namespaces, PVCs, and PVs:

```bash
# This will be blocked if the PVC has a Delete reclaim policy and no snapshot
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
```

## How It Works

pv-safe uses a ValidatingWebhookConfiguration to intercept DELETE operations and applies the following logic:

```
┌─────────────────────────────────────────────────────────┐
│ DELETE Request (Namespace/PVC/PV)                       │
└───────────────────────┬─────────────────────────────────┘
                        │
                        v
┌─────────────────────────────────────────────────────────┐
│ Check bypass label (pv-safe.io/force-delete=true)       │
├─────────────────────────────────────────────────────────┤
│ YES → ALLOW (with audit log)                            │
│ NO  → Continue to risk assessment                       │
└───────────────────────┬─────────────────────────────────┘
                        │
                        v
┌─────────────────────────────────────────────────────────┐
│ Risk Assessment                                          │
├─────────────────────────────────────────────────────────┤
│ 1. PV reclaim policy = Retain?        → ALLOW           │
│ 2. Ready VolumeSnapshot exists?       → ALLOW           │
│ 3. Otherwise                          → BLOCK           │
└─────────────────────────────────────────────────────────┘
```

### Risk Criteria

A deletion is considered **risky** when:
- PersistentVolume has `reclaimPolicy: Delete`, AND
- No ready VolumeSnapshot with `deletionPolicy: Retain` exists

A deletion is considered **safe** when:
- PersistentVolume has `reclaimPolicy: Retain`, OR
- A ready VolumeSnapshot with `deletionPolicy: Retain` exists, OR
- Bypass label `pv-safe.io/force-delete=true` is present

## Examples

### Example 1: Safe Deletion with Snapshot

Create a VolumeSnapshot before deleting:

```bash
# Create a snapshot
kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: my-data-backup
  namespace: production
spec:
  volumeSnapshotClassName: csi-snapclass
  source:
    persistentVolumeClaimName: my-data
EOF

# Wait for snapshot to be ready
kubectl wait --for=jsonpath='{.status.readyToUse}'=true \
  volumesnapshot/my-data-backup -n production --timeout=300s

# Now deletion is allowed
kubectl delete pvc my-data -n production
# persistentvolumeclaim "my-data" deleted
```

### Example 2: Change Reclaim Policy

Make deletion safe by changing the reclaim policy:

```bash
# Get the PV name
PV_NAME=$(kubectl get pvc my-data -n production -o jsonpath='{.spec.volumeName}')

# Change reclaim policy to Retain
kubectl patch pv $PV_NAME -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'

# Now deletion is allowed
kubectl delete pvc my-data -n production
# persistentvolumeclaim "my-data" deleted
```

### Example 3: Force Delete (Data Loss)

When you're certain you want to delete without backup:

```bash
# Add bypass label (explicit acknowledgment of data loss)
kubectl label pvc my-data -n production pv-safe.io/force-delete=true

# Delete the PVC
kubectl delete pvc my-data -n production
# persistentvolumeclaim "my-data" deleted
```

### Example 4: Namespace Deletion

pv-safe checks all PVCs in a namespace:

```bash
$ kubectl delete namespace staging

Error from server (Forbidden): admission webhook "validate.pv-safe.io" denied the request:
DELETION BLOCKED: Namespace 'staging' contains 3 PVC(s) that would lose data permanently

Risky PVCs:
  - postgres-data: PV has Delete reclaim policy, no snapshot found
  - redis-data: PV has Delete reclaim policy, no snapshot found
  - app-cache: PV has Delete reclaim policy, no snapshot found

To safely delete this resource:
  1. Create VolumeSnapshots for the PVCs
  2. OR change PV reclaim policy to Retain for each PVC
  3. OR force delete (will lose data):
     kubectl label namespace staging pv-safe.io/force-delete=true
     kubectl delete namespace staging
```

## Configuration

### Excluded Namespaces

By default, these namespaces are excluded from validation:
- `kube-system`
- `kube-public`
- `kube-node-lease`
- `pv-safe-system`

To modify exclusions, edit `deploy/05-webhook-config.yaml`.

### VolumeSnapshot Support

For VolumeSnapshot support, you need:

1. **Install VolumeSnapshot CRDs:**
```bash
VERSION=v6.3.0
BASE_URL=https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${VERSION}

kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f ${BASE_URL}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml

kubectl apply -f ${BASE_URL}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f ${BASE_URL}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```

2. **Create a VolumeSnapshotClass:**
```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-snapclass
driver: <your-csi-driver>  # e.g., ebs.csi.aws.com, pd.csi.storage.gke.io
deletionPolicy: Retain
```

> **Note:** pv-safe works without VolumeSnapshot CRDs but only uses reclaim policy checks.

## Documentation

- **[Architecture](docs/ARCHITECTURE.md)** - Internal design and how pv-safe works
- **[Development](docs/DEVELOPMENT.md)** - Local setup, testing, and contributing
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Common issues and solutions

## Monitoring

### View Webhook Logs

```bash
# Follow webhook logs
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f

# View blocked deletions
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --since=24h | grep BLOCKING

# View bypass usage
kubectl logs -n pv-safe-system -l app=pv-safe-webhook --since=24h | grep BYPASS
```

### Health Checks

```bash
# Check webhook status
kubectl get pods -n pv-safe-system

# Check webhook configuration
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook
```

## Development

### Local Testing

```bash
# Create a kind cluster with test fixtures
make setup

# Build and deploy webhook
make webhook-build
make webhook-deploy

# View logs
make webhook-logs

# Run tests
make test

# Cleanup
make teardown
```

### Project Structure

```
pv-safe/
├── cmd/webhook/           # Webhook server entry point
├── internal/webhook/      # Core webhook logic
│   ├── handler.go        # Admission request handler
│   ├── risk.go           # Risk assessment engine
│   ├── snapshot.go       # VolumeSnapshot detection
│   └── client.go         # Kubernetes client
├── deploy/               # Kubernetes manifests
├── docs/                 # Documentation
├── scripts/              # Build and deployment scripts
└── test/fixtures/        # Test scenarios
```

## Troubleshooting

### Webhook Not Blocking Deletions

Check if the webhook is running and configured:
```bash
kubectl get pods -n pv-safe-system
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook
kubectl logs -n pv-safe-system -l app=pv-safe-webhook
```

### Snapshot Not Detected

Verify snapshot is ready and has correct deletion policy:
```bash
# Check snapshot status
kubectl get volumesnapshot <name> -n <namespace> -o yaml

# Verify it's ready
kubectl get volumesnapshot <name> -n <namespace> \
  -o jsonpath='{.status.readyToUse}'

# Check snapshot class deletion policy
kubectl get volumesnapshotclass <class-name> -o yaml | grep deletionPolicy
```

### All Deletions Blocked

Check webhook RBAC permissions:
```bash
kubectl auth can-i get pv \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook

kubectl auth can-i list pvc \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook
```

For more troubleshooting, see the [Operator Guide](docs/OPERATOR_GUIDE.md#troubleshooting).

## Uninstallation

### Helm Installation

```bash
helm uninstall pv-safe
```

### Source Installation

```bash
# Using make
make webhook-delete

# Or manually
kubectl delete namespace pv-safe-system
kubectl delete validatingwebhookconfiguration pv-safe-validating-webhook
```

## Architecture

### Components

- **Admission Webhook**: Intercepts DELETE operations via Kubernetes ValidatingWebhookConfiguration
- **Risk Calculator**: Analyzes PV reclaim policies and VolumeSnapshot availability
- **Snapshot Checker**: Queries VolumeSnapshot API (if available) to verify backups exist
- **Handler**: Processes admission requests and generates allow/deny responses

### Security

pv-safe operates with minimal permissions:
- **Read-only** access to PVs, PVCs, Namespaces, and VolumeSnapshots
- **No data modification** capabilities
- **TLS-secured** webhook endpoint (managed by cert-manager)
- **Audit trail** for all bypass operations

### Performance

- **Latency**: Typically <100ms per request
- **Timeout**: 5-second timeout for risk assessment (10-second webhook timeout)
- **Failure Mode**: Configurable (default: fail-closed blocks deletions if webhook is down)

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting PRs.

### Development Setup

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/automationpi/pv-safe/issues)
- **Discussions**: [GitHub Discussions](https://github.com/automationpi/pv-safe/discussions)

## Acknowledgments

- Built with [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
- Inspired by Kubernetes admission webhook best practices
- Certificate management by [cert-manager](https://cert-manager.io/)

---

**Status**: Active Development | **Version**: 0.1.0 | **Kubernetes**: 1.19+
