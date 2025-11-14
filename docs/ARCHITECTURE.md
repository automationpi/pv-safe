# Architecture

This document describes the internal architecture and design decisions of pv-safe.

## Overview

pv-safe is a Kubernetes ValidatingWebhook that intercepts DELETE operations on Namespaces, PersistentVolumeClaims (PVCs), and PersistentVolumes (PVs) to prevent accidental data loss.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes API Server                    │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │ DELETE Request
                      │ (Namespace/PVC/PV)
                      ↓
┌─────────────────────────────────────────────────────────────┐
│         ValidatingWebhookConfiguration                       │
│         (pv-safe-validating-webhook)                        │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │ AdmissionReview Request
                      ↓
┌─────────────────────────────────────────────────────────────┐
│                    pv-safe Webhook Server                    │
│                    (HTTPS on port 8443)                      │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Handler (handler.go)                       │ │
│  │  - Parse AdmissionReview                               │ │
│  │  - Check bypass label                                  │ │
│  │  - Route to RiskCalculator                             │ │
│  └──────────────┬─────────────────────────────────────────┘ │
│                 │                                            │
│                 ↓                                            │
│  ┌────────────────────────────────────────────────────────┐ │
│  │           RiskCalculator (risk.go)                      │ │
│  │  - Get PVC details                                     │ │
│  │  - Get associated PV                                   │ │
│  │  - Check reclaim policy                                │ │
│  │  - Query SnapshotChecker                               │ │
│  └──────────────┬─────────────────────────────────────────┘ │
│                 │                                            │
│                 ↓                                            │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         SnapshotChecker (snapshot.go)                   │ │
│  │  - List VolumeSnapshots (dynamic client)               │ │
│  │  - Check readyToUse status                             │ │
│  │  - Verify VolumeSnapshotClass deletionPolicy           │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │ AdmissionReview Response
                      │ (Allow/Deny + Message)
                      ↓
┌─────────────────────────────────────────────────────────────┐
│                  Kubernetes API Server                       │
│              (Proceeds or Rejects operation)                │
└─────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Handler (`internal/webhook/handler.go`)

**Purpose:** HTTP server and admission request processor

**Responsibilities:**
- Implements `http.Handler` interface
- Parses `AdmissionReview` requests from Kubernetes
- Checks for bypass label (`pv-safe.io/force-delete=true`)
- Routes requests to appropriate risk assessment logic
- Constructs `AdmissionReview` responses
- Provides health check endpoints (`/healthz`, `/readyz`)

**Key Functions:**
- `ServeHTTP()` - Main HTTP handler
- `hasBypassLabel()` - Check for force-delete label
- `handlePVCDeletion()` - Process PVC deletions
- `handleNamespaceDeletion()` - Process namespace deletions

### 2. RiskCalculator (`internal/webhook/risk.go`)

**Purpose:** Risk assessment engine for storage deletions

**Responsibilities:**
- Retrieves PVC and PV details from Kubernetes API
- Evaluates PV reclaim policy (Delete vs Retain)
- Checks for VolumeSnapshot availability
- Determines if deletion is risky or safe
- Generates detailed risk assessment messages

**Decision Logic:**
```
Is PV reclaim policy "Retain"?
  YES → SAFE (data will be preserved)
  NO  → Continue to snapshot check

Does a ready VolumeSnapshot exist with "Retain" deletionPolicy?
  YES → SAFE (backup exists)
  NO  → RISKY (data will be lost)
```

**Key Functions:**
- `AssessPVCDeletion()` - Main risk assessment for PVCs
- `AssessNamespaceDeletion()` - Assess all PVCs in namespace
- `isPVCRisky()` - Core risk logic

### 3. SnapshotChecker (`internal/webhook/snapshot.go`)

**Purpose:** VolumeSnapshot API integration

**Responsibilities:**
- Uses Kubernetes dynamic client to query VolumeSnapshot CRDs
- Handles cases where VolumeSnapshot CRDs are not installed
- Verifies snapshot readiness (`status.readyToUse`)
- Checks VolumeSnapshotClass deletion policy
- Gracefully degrades if snapshot API unavailable

**Key Functions:**
- `NewSnapshotChecker()` - Initialize with CRD availability check
- `HasReadySnapshot()` - Find ready snapshots for a PVC
- `getSnapshotClassDeletionPolicy()` - Get retention policy

**Technical Details:**
- Uses `schema.GroupVersionResource` for VolumeSnapshot API
- Leverages `unstructured.Unstructured` for dynamic access
- Returns `SnapshotInfo` with snapshot details

### 4. Kubernetes Client (`internal/webhook/client.go`)

**Purpose:** Kubernetes API client initialization

**Responsibilities:**
- Creates in-cluster Kubernetes client
- Provides both typed and dynamic clients
- Handles configuration and authentication

## Risk Assessment Algorithm

### For PVC Deletions

```
1. Get PVC from Kubernetes API
   ├─ Error? → ALLOW (fail open for missing resources)
   └─ Success → Continue

2. Get bound PV from PVC spec
   ├─ No PV bound? → ALLOW (no data to lose)
   └─ PV exists → Continue

3. Check PV reclaim policy
   ├─ Policy = "Retain"? → ALLOW (data preserved)
   └─ Policy = "Delete" → Continue to step 4

4. Check for VolumeSnapshots
   ├─ Ready snapshot with "Retain" policy exists? → ALLOW
   └─ No snapshot or "Delete" policy → DENY (RISKY)
```

### For Namespace Deletions

```
1. List all PVCs in namespace
   ├─ No PVCs? → ALLOW (no storage to assess)
   └─ Has PVCs → Continue

2. For each PVC, run PVC risk assessment

3. Collect all risky PVCs

4. If any risky PVCs exist → DENY with list of risky PVCs
   Otherwise → ALLOW
```

## Bypass Mechanism

**Label-Based Bypass:**
- Label: `pv-safe.io/force-delete=true`
- Requires explicit two-step process
- Creates audit trail in webhook logs
- Applies to Namespaces, PVCs, and PVs

**Audit Logging:**
When bypass is used, webhook logs:
```
BYPASS: Force delete label detected
  Resource: <namespace>/<name>
  Type: <PVC/Namespace/PV>
  Allowed: true (bypass label present)
```

## Security Model

### Permissions

pv-safe operates with **read-only** access:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pv-safe-webhook
rules:
  - apiGroups: [""]
    resources:
      - persistentvolumes
      - persistentvolumeclaims
      - namespaces
    verbs:
      - get
      - list
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources:
      - volumesnapshots
      - volumesnapshotclasses
    verbs:
      - get
      - list
```

**Key Security Features:**
- No write/update/delete permissions
- No secret or configmap access
- No modification of any resources
- Read-only observation of state

### TLS Configuration

- **Certificate Management:** cert-manager (automatic rotation)
- **TLS Version:** Minimum TLS 1.2
- **Certificate Location:** `/etc/webhook/certs/`
- **Validity:** 90 days (auto-renewed by cert-manager)

### Failure Mode

**failurePolicy: Fail** (fail-closed)

When webhook is unavailable:
- DELETE operations are blocked
- Prevents data loss even if webhook crashes
- Ensures safety over availability

Trade-off considerations:
- **Fail-closed (current):** Maximum safety, may block legitimate deletions during outages
- **Fail-open (alternative):** Higher availability, reduced safety

## VolumeSnapshot Integration

### Dynamic Client Approach

pv-safe uses Kubernetes dynamic client for VolumeSnapshot access because:
1. **Optional CRD:** VolumeSnapshot CRDs may not be installed
2. **Graceful Degradation:** Works without snapshot support
3. **No Static Dependencies:** Avoids importing snapshot client libraries

### Detection Logic

```go
// Check if VolumeSnapshot CRDs exist
gvr := schema.GroupVersionResource{
    Group:    "snapshot.storage.k8s.io",
    Version:  "v1",
    Resource: "volumesnapshots",
}

// Try to list - fails gracefully if CRD not present
snapshots, err := dynamicClient.Resource(gvr).List(...)
```

### Snapshot Validation Criteria

A snapshot is considered valid for safe deletion if:
1. **Source matches:** `spec.source.persistentVolumeClaimName` matches PVC
2. **Ready state:** `status.readyToUse` is `true`
3. **Retention policy:** VolumeSnapshotClass `deletionPolicy` is `Retain`

## Performance Characteristics

### Latency

**Typical request flow:**
1. Parse admission request: ~1ms
2. Check bypass label: ~0.5ms
3. Get PVC/PV from API: ~10-20ms
4. Check snapshots (if applicable): ~10-30ms
5. Generate response: ~1ms

**Total:** ~50-100ms per request

### Caching Strategy

**Current:** No caching (stateless)

**Rationale:**
- Ensures always-accurate risk assessment
- Avoids stale data issues
- Simplifies implementation
- Acceptable latency for DELETE operations

**Future Consideration:** Short-lived cache (TTL: 5s) for high-load scenarios

### Resource Usage

**Typical deployment:**
- CPU: 50-100m (request), 200m (limit)
- Memory: 64Mi (request), 128Mi (limit)
- Replicas: 2 (high availability)

## High Availability

### Deployment Configuration

```yaml
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
```

### Health Checks

- **Liveness probe:** `/healthz` (ensures process is alive)
- **Readiness probe:** `/readyz` (ensures webhook can serve requests)
- **Timeout:** 5 seconds
- **Period:** 10 seconds

### Webhook Configuration

```yaml
webhooks:
  - name: validate.pv-safe.io
    timeoutSeconds: 10
    failurePolicy: Fail
```

**Timeout Handling:**
- Webhook has 10 seconds to respond
- Internal timeout: 5 seconds for risk assessment
- Buffer: 5 seconds for network/processing

## Design Decisions

### 1. ValidatingWebhook vs MutatingWebhook

**Chosen:** ValidatingWebhook

**Rationale:**
- pv-safe only blocks/allows operations, never modifies
- Validating webhooks are simpler and less risky
- No need for mutation capabilities

### 2. Fail-Closed vs Fail-Open

**Chosen:** Fail-closed (`failurePolicy: Fail`)

**Rationale:**
- Prioritizes data safety over availability
- Prevents data loss during webhook outages
- Acceptable trade-off for DELETE operations

### 3. Label-Based Bypass vs Annotation

**Chosen:** Label-based (`pv-safe.io/force-delete=true`)

**Rationale:**
- Labels are more visible in kubectl output
- Easier to query with label selectors
- Clear intent in Kubernetes ecosystem

### 4. Dynamic Client vs Typed Client for Snapshots

**Chosen:** Dynamic client

**Rationale:**
- VolumeSnapshot CRDs are optional
- Avoids dependency on external client libraries
- Enables graceful degradation
- Simpler deployment (no CRD version conflicts)

### 5. In-Cluster Only vs External Access

**Chosen:** In-cluster only

**Rationale:**
- Webhook must run inside cluster (Kubernetes requirement)
- Simplifies authentication and network configuration
- Uses service account for RBAC

## Extension Points

### Adding Support for New Resource Types

To protect additional resources:

1. Update `ValidatingWebhookConfiguration`:
```yaml
rules:
  - operations: ["DELETE"]
    apiGroups: ["apps"]
    apiVersions: ["v1"]
    resources: ["statefulsets"]
```

2. Add handler in `handler.go`:
```go
case "StatefulSet":
    // Assess StatefulSet PVC templates
```

3. Add risk assessment logic in `risk.go`

### Custom Risk Logic

Extend `RiskCalculator` to support:
- Backup validation (non-VolumeSnapshot)
- Cloud provider snapshot APIs
- Custom retention policies
- Organization-specific rules

### Audit/Logging Integration

Webhook logs to stdout. Integration points:
- Fluentd/Fluent Bit for log aggregation
- Prometheus metrics (future enhancement)
- External audit systems via sidecar

## Testing Strategy

### Unit Tests

**Focus:** Individual component logic
- Risk assessment calculations
- Bypass label detection
- Snapshot matching logic

### Integration Tests

**Focus:** End-to-end webhook behavior
- Deploy webhook to kind cluster
- Create test PVCs and snapshots
- Attempt deletions, verify blocking

**Test Fixtures:** `test/fixtures/`
- Risky PVCs (Delete policy, no snapshot)
- Safe PVCs (Retain policy or with snapshot)
- Namespace deletion scenarios

### Manual Testing

**Makefile targets:**
```bash
make setup              # Create kind cluster
make webhook-build      # Build webhook image
make webhook-deploy     # Deploy to cluster
make test-fixtures-apply  # Apply test resources
```

## Future Enhancements

### Planned Features

1. **Prometheus Metrics**
   - Deletions blocked/allowed counters
   - Risk assessment latency histogram
   - Bypass usage tracking

2. **Webhook for StatefulSets**
   - Check PVC templates
   - Assess volumeClaimTemplates

3. **Backup Verification**
   - Support Velero backups
   - Cloud provider snapshot APIs
   - Custom backup integrations

4. **Dry-Run Mode**
   - Log warnings without blocking
   - Helpful for initial rollout

5. **Notification Integration**
   - Slack/email alerts for blocks
   - Audit trail to external systems

### Performance Optimizations

1. **Caching Layer**
   - Short-lived cache for PV details
   - Reduce API server load

2. **Batch Operations**
   - Optimize namespace deletion checks
   - Parallel PVC assessments

3. **Informer-Based Watching**
   - Maintain local cache of resources
   - Reduce API latency
