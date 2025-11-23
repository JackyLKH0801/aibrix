# Multi-Tenancy in AIBrix

AIBrix supports multi-tenant model deployments, allowing you to share model resources securely and efficiently across different teams or users. This document outlines how to configure, observe, and manage multi-tenant deployments.

## Configuration

Multi-tenancy is configured via the `ModelAdapter` CRD. You can specify authentication requirements and isolation policies for each model adapter.

### ModelAdapter Spec

The `ModelAdapter` spec has been extended with `AuthConfig` and `IsolationPolicy` fields.

```yaml
apiVersion: model.aibrix.ai/v1alpha1
kind: ModelAdapter
metadata:
  name: llama-2-7b
spec:
  # ... existing fields ...
  
  # Authentication Configuration
  authConfig:
    enabled: true
    provider: "jwt" # "jwt" or "oauth2"
    issuer: "https://keycloak.example.com/..."
    secretRef:
      name: "auth-secret"
      key: "client-secret"

  # Isolation Policy
  isolationPolicy:
    strategy: "namespace" # or "pod"
    namespace: "tenant-a-ns" # if strategy is namespace
    nodeSelector:
      tenant: "tenant-a"
    tolerations:
      - key: "tenant"
        operator: "Equal"
        value: "tenant-a"
        effect: "NoSchedule"
```

### Fields Description

- **authConfig**: Configures authentication for the model adapter.
    - `enabled`: Enable or disable authentication.
    - `provider`: The auth provider type (`jwt`, `oauth2`, `none`).
    - `issuer`: The expected issuer for JWT tokens.
    - `audience`: The expected audience for JWT tokens.
    - `secretRef`: Reference to a Kubernetes Secret containing authentication credentials.

- **isolationPolicy**: Configures resource isolation.
    - `strategy`: The isolation strategy. `namespace` isolates resources in a specific namespace. `pod` allows sharing the namespace but isolates at the pod level.
    - `namespace`: The target namespace for the model pods (required if strategy is `namespace`).
    - `nodeSelector`: Node labels to select specific nodes for the model pods.
    - `tolerations`: Taints and tolerations for scheduling.

## Observability

AIBrix provides built-in observability for multi-tenant workloads.

### Metrics

The Gateway emits the following metrics with a `tenant_id` label:

- `aibrix_gateway_requests_total`: Total number of requests processed by the gateway.
- `aibrix_gateway_request_duration_seconds`: Histogram of request latencies.
- `aibrix_gateway_request_errors_total`: Total number of failed requests.

### Grafana Dashboard

A dedicated Grafana dashboard "AIBrix Multi-Tenant Dashboard" is available to visualize these metrics. It provides:

- **Request Rate per Tenant**: Monitor traffic volume for each tenant.
- **Error Rate per Tenant**: Identify tenants experiencing high error rates.
- **Latency Distribution**: Analyze performance per tenant.
- **Resource Usage**: (If configured) Track GPU/CPU usage per tenant.

To import the dashboard, use the JSON file located at `observability/grafana/AIBrix_Multi_Tenant_Dashboard.json`.

## Migration Guide

To enable multi-tenancy on existing `ModelAdapter` resources:

1.  **Upgrade CRDs**: Ensure the `ModelAdapter` CRD is updated to the latest version containing the `auth` and `isolation` fields.
2.  **Update Resources**: Edit your `ModelAdapter` manifests to include the desired `auth` and `isolation` configurations.
3.  **Apply Changes**: Apply the updated manifests using `kubectl apply`.

Example patch to enable namespace isolation for an existing adapter:

```yaml
spec:
  isolation:
    strategy: "namespace"
    namespace: "new-tenant-ns"
```

## Best Practices

- **Namespace Isolation**: For strict security requirements, use `namespace` isolation to separate tenant resources completely.
- **Resource Quotas**: Use Kubernetes ResourceQuotas in tenant namespaces to limit resource consumption.
- **Monitoring**: Set up alerts based on the per-tenant metrics to detect issues early.
