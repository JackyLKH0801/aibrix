# Troubleshooting Guide

## ModelAdapter Loading Failure

### Symptom
The `ModelAdapter` resource stays in `Bound: False` state with reason `ModelAdapterLoadingError`.
The message says "Failed to load ModelAdapter ...: max retries exceeded".

### Cause
This usually happens when the vLLM pod cannot download the adapter artifact from the specified URL.
Common causes:
1. **Network Connectivity**: The cluster nodes or pods do not have internet access to reach HuggingFace or other model repositories.
2. **DNS Resolution**: The pod cannot resolve the hostname (e.g., `huggingface.co`).
3. **Authentication**: The repository requires authentication (e.g., private HuggingFace model) and the secret is missing or incorrect.

### Debugging
Check the controller logs:
```bash
kubectl logs -n aibrix-system -l app=aibrix-controller-manager
```

Check the vLLM pod logs:
```bash
kubectl logs <vllm-pod-name>
```
Look for errors like `NameResolutionError`, `ConnectionRefused`, or `401 Unauthorized`.

### Solution

#### 1. Fix Network/DNS
Ensure your cluster has internet access. If you are behind a proxy, configure the proxy settings in the vLLM deployment.

#### 2. Use Local Models
If internet access is not available, you can download the model artifact manually and host it in a local object store (e.g., MinIO) or mount it to the pod.

To use a local path (mounted volume):
1. Update the vLLM Deployment to mount the model directory.
2. Use `file://` scheme in `artifactURL` (if supported) or ensure the path is accessible.
*Note: The current implementation primarily supports `huggingface://` and `s3://`.*

#### 3. Configure Authentication
For private models, ensure you have created the secret and referenced it in `authConfig` or `credentialsSecretRef`.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hf-secret
type: Opaque
data:
  token: <base64-encoded-token>
```

## Controller Compilation/Run Issues

### "no kind is registered for the type v1.HTTPRoute"
This error occurs if the Gateway API CRDs are not installed or the types are not registered in the controller's scheme.
Ensure Gateway API CRDs are installed:
```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml
```
And ensure the controller imports and registers `sigs.k8s.io/gateway-api/apis/v1`.
