# Multi-Tenancy Sample

This sample demonstrates how to configure and test multi-tenancy in AIBrix.

## Prerequisites

1.  A running Kubernetes cluster with AIBrix installed.
2.  `kubectl` configured to talk to the cluster.
3.  `curl` installed.

## Steps

### 1. Update CRDs

Ensure the `ModelAdapter` CRD in your cluster supports the new `auth` and `isolation` fields.

```bash
make manifests
kubectl apply -f config/crd/bases/model.aibrix.ai_modeladapters.yaml
```

### 2. Deploy the Model Adapter

Apply the sample ModelAdapter with tenant configuration.

```bash
kubectl apply -f samples/multitenancy/model-adapter-tenant.yaml
```

### 3. Port Forward Gateway

Expose the AIBrix Gateway locally.

```bash
kubectl port-forward svc/envoy-gateway -n envoy-gateway-system 8888:80
```

### 4. Run Test Script

Run the test script to send requests with different Tenant IDs.

```bash
chmod +x samples/multitenancy/test-script.sh
./samples/multitenancy/test-script.sh
```

### 5. Verify Metrics

1.  Port forward Grafana:
    ```bash
    kubectl port-forward svc/grafana -n monitoring 3000:3000
    ```
2.  Open Grafana at `http://localhost:3000`.
3.  Import the dashboard from `observability/grafana/AIBrix_Multi_Tenant_Dashboard.json`.
4.  Verify that you see traffic for `tenant-a`, `tenant-b`, and `default`.
