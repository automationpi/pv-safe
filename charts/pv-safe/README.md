# pv-safe Helm Chart

This Helm chart installs pv-safe, a Kubernetes admission webhook that prevents accidental data loss from PersistentVolume deletions.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- cert-manager (optional but recommended)

## Installing the Chart

### With cert-manager (Recommended)

If you don't have cert-manager installed:

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager -n cert-manager
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager-webhook -n cert-manager
```

Then install pv-safe:

```bash
helm install pv-safe ./charts/pv-safe
```

### Without cert-manager

If you manage certificates externally:

```bash
helm install pv-safe ./charts/pv-safe \
  --set certificate.enabled=false \
  --set webhook.caBundle=<your-ca-bundle-base64>
```

## Uninstalling the Chart

```bash
helm uninstall pv-safe
```

## Configuration

The following table lists the configurable parameters of the pv-safe chart and their default values.

### Webhook Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `webhook.replicaCount` | Number of webhook replicas | `2` |
| `webhook.image.repository` | Webhook image repository | `ghcr.io/automationpi/pv-safe` |
| `webhook.image.tag` | Webhook image tag | `Chart.AppVersion` |
| `webhook.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `webhook.port` | Webhook server port | `8443` |
| `webhook.resources.limits.cpu` | CPU limit | `200m` |
| `webhook.resources.limits.memory` | Memory limit | `128Mi` |
| `webhook.resources.requests.cpu` | CPU request | `100m` |
| `webhook.resources.requests.memory` | Memory request | `64Mi` |

### RBAC Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbac.create` | Create RBAC resources | `true` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | Generated from fullname |

### Certificate Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `certificate.enabled` | Use cert-manager for certificates | `true` |
| `certificate.issuer.create` | Create self-signed issuer | `true` |
| `certificate.issuer.kind` | Issuer kind (Issuer or ClusterIssuer) | `Issuer` |
| `certificate.duration` | Certificate duration | `8760h` (1 year) |
| `certificate.renewBefore` | Renew before expiry | `720h` (30 days) |

### ValidatingWebhook Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `validatingWebhook.failurePolicy` | Failure policy (Fail or Ignore) | `Fail` |
| `validatingWebhook.timeoutSeconds` | Webhook timeout in seconds | `10` |
| `validatingWebhook.additionalExcludedNamespaces` | Additional namespaces to exclude | `[]` |

### Namespace Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `namespace.create` | Create the namespace | `true` |
| `namespace.name` | Namespace name | `pv-safe-system` |

## Examples

### Custom Configuration

```bash
helm install pv-safe ./charts/pv-safe \
  --set webhook.replicaCount=3 \
  --set validatingWebhook.failurePolicy=Ignore \
  --set webhook.resources.limits.memory=256Mi
```

### Using values.yaml

Create a `custom-values.yaml`:

```yaml
webhook:
  replicaCount: 3
  resources:
    limits:
      cpu: 500m
      memory: 256Mi
    requests:
      cpu: 200m
      memory: 128Mi

validatingWebhook:
  failurePolicy: Ignore
  additionalExcludedNamespaces:
    - dev-*
    - test-*

nodeSelector:
  node-role.kubernetes.io/control-plane: ""
```

Install with custom values:

```bash
helm install pv-safe ./charts/pv-safe -f custom-values.yaml
```

### Upgrade

```bash
helm upgrade pv-safe ./charts/pv-safe \
  --set webhook.image.tag=0.2.0
```

## Troubleshooting

### Check webhook status

```bash
kubectl get pods -n pv-safe-system
kubectl logs -n pv-safe-system -l app=pv-safe-webhook
```

### Check certificate

```bash
kubectl get certificate -n pv-safe-system
kubectl describe certificate pv-safe-webhook-cert -n pv-safe-system
```

### Verify webhook configuration

```bash
kubectl get validatingwebhookconfiguration pv-safe-validating-webhook -o yaml
```

## More Information

- [Operator Guide](../../docs/OPERATOR_GUIDE.md)
- [Quick Reference](../../docs/QUICK_REFERENCE.md)
- [GitHub Repository](https://github.com/automationpi/pv-safe)
