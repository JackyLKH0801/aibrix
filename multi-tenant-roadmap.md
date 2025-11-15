# Multi-Tenant Model Deployments & Tenant-Aware Routing

This document captures the engineering to-do list for implementing RFC #1101, enabling isolated multi-tenant deployments and routing in AIBrix.

## 1. Define Tenant Metadata and Routing Keys

### 1.1 Canonical headers, claims, and fallbacks
- Reserve `X-Tenant-ID` (required), `X-Deployment-ID` (optional override), and `X-Tenant-Metadata` (JSON blob for LoRA/auth context) as the primary transport headers.
    -> type.go defines X-Tenant-ID, X-Deployment-ID, X-Tenant-Metadata
- Accept JWT claims `tenant_id`, `deployment_id`, `priority` as equivalents when requests are authenticated via bearer tokens; document precedence rules (headers override claims unless forbidden by policy).
    -> gateway_req_headers.go: extractTenantMetadata allow both header/ JWT claim
- Add a compatibility shim: if neither header nor claim exists, route through a "legacy" tenant `default` and emit warnings/metrics so existing integrations keep working during migration.
    -> tenent.go: implement detectTenantConflicts and 
- Publish these expectations in the public API doc and add request validation that returns `400` when conflicting IDs are supplied.
    ->gateway_req_headers.go: validateTenantMetadata to check if it is valid or not

### 1.2 Composite cache key + API/Proto updates
- Define the canonical cache key as `<tenant_id>::<model_id>::<deployment_rev>` where `deployment_rev` hashes spec + LoRA adapters to guarantee uniqueness after rollouts.
    -> use 3 parameter: tenantID, modelID, deploymentRev to build a composite key
- Update internal proto structs (e.g., `RoutingContext`, `CachedTarget`) with explicit `tenant_id`, `model_id`, `deployment_rev` fields instead of relying on free-form strings.
- Introduce helper utilities in the gateway package to build/parse the composite key consistently and add unit-tests covering edge cases (UTF-8 tenants, missing deployment rev, etc.).
    -> tenent.go: func buildCompositeKey, parsebuildCompositeKey
- Version the API so controllers and plugins can adopt the richer metadata without breaking older agents; provide feature-gate to toggle composite routing.
    

### 1.3 CRD extensions and validation
- Extend Model/Deployment/LoRAAdapter CRDs with `spec.tenant` (string), `spec.routingHeaders` (list) and `status.tenantHash` to make tenancy first-class.
- Add OpenAPI validation to ensure `(namespace, tenant, deploymentName)` tuples remain unique and disallow empty tenant IDs outside of the legacy `default` tenant.
- Surface tenant metadata in generated pod labels/annotations (`tenant.aibrix.ai/id`, `tenant.aibrix.ai/deployment`) so downstream informers and autoscalers can filter efficiently.
- Provide kubebuilder defaults/webhooks to auto-fill tenant fields from namespace annotations when operators omit them, keeping UX simple while preserving isolation.

## 2. Extend Gateway Routing & Authentication Logic
- Introduce tenant-aware auth middleware in `gateway.go` (OAuth/JWT validation, key management, per-tenant rate/SLA policies).
- Update `HandleRequestBody` and `selectTargetPod` to compute composite keys, hit a tenant→model cache layer, and emit tenant context in logs/metrics.
- Ensure Envoy receives tenant-scoped `x-target-pod` headers and honor routing failures with clear 4xx/5xx semantics.
- Add configuration knobs for cache TTLs, eviction strategy, and hierarchical vs label-based routing modes.

## 3. Update Controllers, Pod Labeling & HTTPRoutes
- Modify the model adapter/controller to stamp pods with tenant + model labels/annotations and enforce namespace/quotas for isolation.
- Generate tenant-specific `HTTPRoute` objects that match composite headers (`X-Tenant-ID`, `X-Model-Name`) and avoid cross-tenant routing collisions.
- Teach informer caches to index pods by composite key so the gateway lookup remains O(1) even as tenants scale.

## 4. Add Isolation-Aware Autoscaling & LoRA Handling
- Feed autoscalers with tenant-scoped metrics (QPS, p99 latency, GPU/KV cache usage) and scale pods per tenant without affecting others.
- Prevent KV cache reuse or LoRA adapter loading across tenants by leveraging adapter metadata + dedicated volumes or namespaces.
- Support tenant-specific scaling policies (min/max replicas, burst limits) and document the reconciliation order when multiple tenants share a base model image.

## 5. Observability, Configuration & Testing
- Emit tenant labels in metrics/logs/traces (Prometheus, OTLP) and add dashboards to visualize per-tenant health/SLOs.
- Provide configuration surfaces (CRDs or ConfigMaps) for tenant auth providers, routing strategies, and isolation policies.
- Add integration tests covering routing, cache eviction, autoscaling, and failure scenarios (missing tenant header, unauthorized access, cache miss).
- Document rollout steps, migration guidance for existing single-tenant deployments, and KPIs to validate before GA.
