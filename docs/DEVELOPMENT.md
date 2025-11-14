# Development Guide

This guide covers local development setup, testing, and contribution workflows for pv-safe.

## Prerequisites

### Required Tools

- **Go 1.21+** - [Install Go](https://golang.org/doc/install)
- **Docker** - [Install Docker](https://docs.docker.com/get-docker/)
- **kind** - [Install kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
- **kubectl** - [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **make** - Usually pre-installed on Linux/macOS

### Optional Tools

- **Helm 3** - For testing Helm chart
- **golangci-lint** - For local linting
- **cert-manager** - Auto-installed by scripts

## Quick Start

### 1. Clone Repository

```bash
git clone https://github.com/automationpi/pv-safe.git
cd pv-safe
```

### 2. Set Up Development Cluster

Create a local kind cluster with all dependencies:

```bash
make setup
```

This command:
- Creates a kind cluster named `pv-safe-dev`
- Installs cert-manager
- Installs VolumeSnapshot CRDs and controller
- Creates test namespaces

### 3. Build and Deploy Webhook

```bash
# Build webhook Docker image
make webhook-build

# Deploy to kind cluster
make webhook-deploy
```

### 4. Apply Test Fixtures

```bash
# Create test PVCs and snapshots
make test-fixtures-apply
```

### 5. Test Webhook

Try deleting a risky PVC:

```bash
kubectl delete pvc database-data -n test-risky
```

Expected output:
```
Error from server (Forbidden): admission webhook "validate.pv-safe.io" denied the request:
DELETION BLOCKED: PVC 'test-risky/database-data' would lose data permanently
```

### 6. View Logs

```bash
# Follow webhook logs
make webhook-logs

# Or use kubectl directly
kubectl logs -n pv-safe-system -l app=pv-safe-webhook -f
```

## Development Workflow

### Making Code Changes

1. **Create a feature branch:**
```bash
git checkout -b feature/your-feature
```

2. **Make changes to Go code:**
   - Core webhook logic: `internal/webhook/`
   - Entry point: `cmd/webhook/main.go`

3. **Rebuild and redeploy:**
```bash
make webhook-build
make webhook-deploy
```

4. **Test your changes:**
```bash
# Apply test fixtures
make test-fixtures-apply

# Test deletions
kubectl delete pvc <test-pvc> -n <namespace>

# Check logs
make webhook-logs
```

5. **Run tests:**
```bash
# Run Go unit tests
go test ./...

# Run with coverage
go test -v -race -coverprofile=coverage.txt ./...
```

### Iterative Development

For faster iteration during development:

```bash
# Watch mode - rebuild on changes (requires 'entr')
find . -name '*.go' | entr -r make webhook-build webhook-deploy
```

Or manually:

```bash
# Edit code
vim internal/webhook/handler.go

# Rebuild
make webhook-build

# Redeploy (faster than full deploy)
kubectl rollout restart deployment/pv-safe-webhook -n pv-safe-system

# Watch pods restart
kubectl get pods -n pv-safe-system -w
```

## Project Structure

```
pv-safe/
├── cmd/
│   └── webhook/
│       └── main.go              # Webhook server entry point
├── internal/
│   └── webhook/
│       ├── client.go            # Kubernetes client setup
│       ├── handler.go           # Admission request handler
│       ├── risk.go              # Risk assessment engine
│       └── snapshot.go          # VolumeSnapshot integration
├── charts/
│   └── pv-safe/                 # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── scripts/
│   ├── create-cluster.sh        # Create kind cluster
│   ├── build-webhook.sh         # Build Docker image
│   ├── deploy-webhook.sh        # Deploy webhook
│   └── apply-fixtures.sh        # Apply test fixtures
├── test/
│   └── fixtures/                # Test YAML files
│       ├── 00-namespaces.yaml
│       ├── 01-risky-pvcs-and-pods.yaml
│       ├── 02-safe-pvcs-and-pods.yaml
│       └── ...
├── .github/
│   └── workflows/               # CI/CD pipelines
├── docs/                        # Documentation
├── Dockerfile                   # Multi-stage build
├── Makefile                     # Development commands
└── go.mod                       # Go module definition
```

## Testing

### Unit Tests

**Run all tests:**
```bash
go test ./...
```

**Run with verbose output:**
```bash
go test -v ./...
```

**Run specific package:**
```bash
go test ./internal/webhook
```

**Generate coverage report:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

### Integration Tests

**Manual integration testing:**

1. **Deploy webhook:**
```bash
make setup
make webhook-build
make webhook-deploy
```

2. **Apply test fixtures:**
```bash
make test-fixtures-apply
```

3. **Test scenarios:**

```bash
# Scenario 1: Block risky PVC deletion
kubectl delete pvc database-data -n test-risky
# Expected: Blocked

# Scenario 2: Allow PVC with snapshot
kubectl delete pvc backup-data -n test-safe
# Expected: Allowed (has snapshot)

# Scenario 3: Allow with bypass label
kubectl label pvc database-data -n test-risky pv-safe.io/force-delete=true
kubectl delete pvc database-data -n test-risky
# Expected: Allowed (bypass)

# Scenario 4: Block namespace with risky PVCs
kubectl delete namespace staging
# Expected: Blocked (contains risky PVCs)
```

4. **Verify webhook logs:**
```bash
kubectl logs -n pv-safe-system -l app=pv-safe-webhook | grep -E "(BLOCKING|ALLOWING|BYPASS)"
```

### Test Fixtures

Test fixtures are in `test/fixtures/`:

- `00-namespaces.yaml` - Test namespaces
- `01-risky-pvcs-and-pods.yaml` - PVCs with Delete reclaim policy, no snapshots
- `02-safe-pvcs-and-pods.yaml` - PVCs with Retain reclaim policy
- `05-volumesnapshotclass.yaml` - VolumeSnapshotClass with Retain policy
- `06-volumesnapshot-test.yaml` - Ready snapshots for safe PVCs

**Apply all fixtures:**
```bash
make test-fixtures-apply
```

**Clean up fixtures:**
```bash
make test-fixtures-cleanup
```

## Debugging

### Enable Verbose Logging

Edit webhook deployment to add debug flags:

```bash
kubectl edit deployment pv-safe-webhook -n pv-safe-system
```

Add environment variable:
```yaml
env:
  - name: DEBUG
    value: "true"
```

### Debug Webhook Locally

For deeper debugging, run webhook outside cluster:

1. **Get kubeconfig from kind:**
```bash
kind get kubeconfig --name pv-safe-dev > kubeconfig-kind.yaml
```

2. **Generate TLS certificates:**
```bash
# Use cert-manager certificates from cluster
kubectl get secret pv-safe-webhook-tls -n pv-safe-system -o jsonpath='{.data.tls\.crt}' | base64 -d > tls.crt
kubectl get secret pv-safe-webhook-tls -n pv-safe-system -o jsonpath='{.data.tls\.key}' | base64 -d > tls.key
```

3. **Run webhook locally:**
```bash
KUBECONFIG=kubeconfig-kind.yaml go run cmd/webhook/main.go \
  --cert-file=tls.crt \
  --key-file=tls.key \
  --port=8443
```

4. **Update webhook configuration to point to localhost:**
```bash
kubectl patch validatingwebhookconfiguration pv-safe-validating-webhook \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/clientConfig/url", "value": "https://host.docker.internal:8443/validate"}]'
```

### Common Issues

**Issue:** Webhook not blocking deletions

**Debug steps:**
```bash
# Check webhook is running
kubectl get pods -n pv-safe-system

# Check webhook configuration
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook -o yaml

# Check webhook logs
kubectl logs -n pv-safe-system -l app=pv-safe-webhook

# Test webhook endpoint
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -k https://pv-safe-webhook.pv-safe-system.svc:443/healthz
```

**Issue:** Certificate errors

**Debug steps:**
```bash
# Check cert-manager is running
kubectl get pods -n cert-manager

# Check certificate status
kubectl get certificate -n pv-safe-system

# Check certificate details
kubectl describe certificate pv-safe-webhook-cert -n pv-safe-system

# Manually trigger cert renewal
kubectl delete certificate pv-safe-webhook-cert -n pv-safe-system
# Certificate will be auto-recreated
```

**Issue:** Snapshot not detected

**Debug steps:**
```bash
# Check if VolumeSnapshot CRDs are installed
kubectl get crd | grep snapshot

# Check snapshot status
kubectl get volumesnapshot -n <namespace>

# Check snapshot details
kubectl describe volumesnapshot <name> -n <namespace>

# Check webhook has RBAC for snapshots
kubectl auth can-i get volumesnapshots \
  --as=system:serviceaccount:pv-safe-system:pv-safe-webhook \
  -n <namespace>
```

## Code Style and Linting

### Go Formatting

**Format code:**
```bash
gofmt -w .
```

**Check formatting:**
```bash
gofmt -d .
```

### Linting

**Install golangci-lint:**
```bash
# macOS
brew install golangci-lint

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

**Run linter:**
```bash
golangci-lint run
```

**Auto-fix issues:**
```bash
golangci-lint run --fix
```

### Code Quality

**Check for common issues:**
```bash
go vet ./...
```

**Check for race conditions:**
```bash
go test -race ./...
```

**Static analysis:**
```bash
staticcheck ./...
```

## Building and Releasing

### Build Docker Image

**Local build:**
```bash
docker build -t pv-safe-webhook:dev .
```

**Multi-arch build:**
```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/automationpi/pv-safe:dev \
  --push .
```

### Test Helm Chart

**Lint chart:**
```bash
helm lint charts/pv-safe
```

**Template chart (dry-run):**
```bash
helm template pv-safe charts/pv-safe --debug
```

**Install chart:**
```bash
helm install pv-safe charts/pv-safe \
  --set webhook.image.tag=dev \
  --set webhook.image.pullPolicy=Never
```

### Release Process

See [CONTRIBUTING.md](../CONTRIBUTING.md) for full release process.

**Summary:**
1. Update `charts/pv-safe/Chart.yaml` versions
2. Update `CHANGELOG.md`
3. Commit changes
4. Tag release: `git tag -a v0.x.0 -m "Release v0.x.0"`
5. Push tag: `git push origin v0.x.0`
6. GitHub Actions will build and publish automatically

## Useful Makefile Targets

```bash
# Cluster management
make setup              # Create kind cluster with dependencies
make teardown           # Delete kind cluster

# Webhook development
make webhook-build      # Build webhook Docker image
make webhook-deploy     # Deploy webhook to cluster
make webhook-delete     # Remove webhook from cluster
make webhook-logs       # View webhook logs
make webhook-status     # Check webhook pod status

# Testing
make test               # Run Go unit tests
make test-fixtures-apply   # Apply test fixtures
make test-fixtures-cleanup # Remove test fixtures

# Utilities
make clean              # Clean build artifacts
make help               # Show available targets
```

## Environment Variables

**Development:**
- `CLUSTER_NAME` - Kind cluster name (default: `pv-safe-dev`)
- `KUBECONFIG` - Path to kubeconfig file
- `DEBUG` - Enable debug logging in webhook

**Build:**
- `IMAGE_NAME` - Docker image name (default: `pv-safe-webhook`)
- `IMAGE_TAG` - Docker image tag (default: `latest`)

## Contributing

Before submitting a PR:

1. **Run tests:**
```bash
go test ./...
```

2. **Check formatting:**
```bash
gofmt -d .
```

3. **Run linter:**
```bash
golangci-lint run
```

4. **Test Helm chart:**
```bash
helm lint charts/pv-safe
helm template pv-safe charts/pv-safe --debug
```

5. **Update documentation if needed**

6. **Follow commit message guidelines** (see [CONTRIBUTING.md](../CONTRIBUTING.md))

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed contribution guidelines.

## Getting Help

- **Documentation:** [docs/](.)
- **Issues:** [GitHub Issues](https://github.com/automationpi/pv-safe/issues)
- **Discussions:** [GitHub Discussions](https://github.com/automationpi/pv-safe/discussions)

## Resources

### Kubernetes Admission Webhooks

- [Dynamic Admission Control](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
- [Webhook Best Practices](https://kubernetes.io/docs/reference/access-authn-authz/webhook/)

### VolumeSnapshot API

- [Volume Snapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/)
- [CSI Snapshotter](https://github.com/kubernetes-csi/external-snapshotter)

### Testing with kind

- [kind Documentation](https://kind.sigs.k8s.io/)
- [Testing Kubernetes Webhooks Locally](https://kubernetes.io/blog/2018/09/24/kubernetes-in-docker/)
