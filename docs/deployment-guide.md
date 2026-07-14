# Deployment Guide

This document lists hard prerequisites and provides step-by-step deployment instructions for each component required to run the AI Workload Controller, sidecar, and optional Tetragon observability.

If you are doing local testing, prefer `kind` or `minikube` with multi-node support for Tetragon verification. For production, follow your customary cluster hardening and node provisioning practices before installing these components.

## Detailed Prerequisites

- **Kubernetes cluster**
  - Minimum: Kubernetes `v1.27`.
  - Access: kubeconfig with `cluster-admin` or equivalent permissions for installing CRDs, RBAC, and webhooks.
  - Nodes should support eBPF (modern Linux kernels 5.4+ recommended) if you plan to use Tetragon.

- **Container runtime & registry**
  - Docker or a compatible builder (buildx) to build images.
  - Registry access (Docker Hub, GCR, ECR, private registry). Ensure nodes can pull images from the chosen registry.

- **CNI / Tetragon**
  - Tetragon is packaged with Cilium charts; if you plan to use Tetragon, prefer the Cilium CNI or verify your CNI supports required BPF features.
  - Kernel requirements: recent kernel with BPF tracing support. See Tetragon upstream docs for exact kernel/compiler constraints.

- **Helm 3**
  - Required for installing Tetragon and the operator Helm chart.

- **Policy engine (optional for additional enforcement)**
  - OPA Gatekeeper or Kyverno can be used to run additional cluster policy checks; the operator does not depend on them but examples assume one is present for policy-as-code extensions.

- **Telemetry backend**
  - An OpenTelemetry collector (OTel) + a storage backend (Elasticsearch, SIEM, or other) to receive `semantic` and `behavioral` signals.
  - Configure indexes so semantic and behavioral signals are stored separately for correlation.

- **Certificates / DNS for webhook**
  - The admission webhook requires a TLS endpoint reachable by the API server. You can use cert-manager to manage the webhook certificates, or generate a self-signed CA and patch the `MutatingWebhookConfiguration`/`ValidatingWebhookConfiguration` as needed.

- **RBAC / ServiceAccount**
  - The operator needs a ServiceAccount with RBAC to manage CRDs and status subresources. The provided `config/rbac/role.yaml` is the starting point.

## Deployment Steps by Requirement

Below are the concrete steps and verification commands for each component. Replace `<registry>` and `<namespace>` as appropriate.

### 1) Build & publish images (sidecar and operator)

Commands:

```bash
# Build sidecar
cd sidecar
go build ./...        # optional sanity
docker build -t <registry>/semantic-guardrail:latest .
docker push <registry>/semantic-guardrail:latest

# Build operator
cd ../
docker build -t <registry>/ai-workload-controller:latest .
docker push <registry>/ai-workload-controller:latest
```

Verification:

```bash
docker pull <registry>/semantic-guardrail:latest
docker pull <registry>/ai-workload-controller:latest
```

Expected: both images pull successfully from your registry.

### 2) Install Tetragon (optional - for behavioral signals)

Commands:

```bash
helm repo add cilium https://helm.cilium.io
helm repo update
helm install tetragon cilium/tetragon -n kube-system --create-namespace
kubectl apply -f deploy/tetragon/agent-egress-baseline.yaml
```

Verification:

```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=tetragon
kubectl logs -n kube-system -l app.kubernetes.io/name=tetragon --tail=50
```

Expected: Tetragon pods `Running` on each node. Logs show `tcp_connect`/`execve` events when test workloads exercise network/exec behavior.

Notes & troubleshooting:
  - If pods crash, check kernel features (`bpftool`, `dmesg`) and CNI compatibility.

### 3) Install CRDs, RBAC, and the Operator

Commands:

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/role.yaml
helm install ai-workload-controller charts/ai-workload-controller \
  -n ai-security --create-namespace \
  --set image.repository=<registry>/ai-workload-controller
```

Alternative: run controller locally for development:

```bash
# from repo root
go run ./main.go --metrics-bind-address=:8080 --health-probe-bind-address=:8081
```

Verification:

```bash
kubectl get deployment -n ai-security
kubectl get crd | grep security.internal
kubectl get aipolicy --all-namespaces
```

Expected: CRDs present; operator Deployment or local process running.

### 4) Provision Admission Webhook (TLS + registration)

Commands (cert-manager example):

```bash
# Install cert-manager if not present
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml

# Create a Certificate/Issuer for the webhook and then apply manifests
kubectl apply -f config/webhook/manifests.yaml
```

Verification:

```bash
kubectl get validatingwebhookconfiguration
kubectl describe validatingwebhookconfiguration ai-workload-controller-webhook
```

Expected: webhook registered and `CABundle` configured (cert-manager will fill this when using a proper Issuer).

### 5) Apply sample policies and example agent

Commands:

```bash
kubectl create namespace ai-agents || true
kubectl apply -f config/samples/aipolicy_sample.yaml
kubectl apply -f config/samples/semanticbudget_sample.yaml
kubectl apply -f deploy/examples/agent-deployment.yaml
```

Verification:

```bash
kubectl get aipolicy -n ai-agents
kubectl get semanticbudget -n ai-agents
kubectl get pods -n ai-agents
kubectl describe aipolicy support-agent-policy -n ai-agents
```

Expected: `AIPolicy` shows `Accepted=True`, `SemanticBudget` status fields present, agent pod(s) running with sidecar container.

### 6) Sidecar wiring and runtime aggregator URL

What to set in your agent Deployment (see `deploy/examples/agent-deployment.yaml`):

- Add the sidecar container image `semantic-guardrail` alongside your agent.
- Ensure environment variables:
  - `RUNTIME_AGGREGATOR_URL`: `http://<operator-or-aggregator-service>:9443/v1/verdicts`
  - `AGENT_IDENTITY`: should match `AIPolicy.spec.identity` (e.g., `support-agent`).

Verification:

```bash
kubectl logs -n ai-agents deploy/agent-workload -c semantic-guardrail --tail=100
kubectl exec -n ai-agents deploy/agent-workload -c semantic-guardrail -- curl -sS localhost:9090/healthz
```

Expected: health endpoint responds and the sidecar reports verdicts to the runtime aggregator when simulated high-risk calls occur.

### 7) Policy-as-code tests

Commands:

```bash
opa test policies/rego -v
```

Expected: all provided tests pass. Add new unit tests as you extend `ai_policy.rego`.

### 8) Telemetry pipeline

Instructions:

- Configure your OTel collector to receive spans/metrics from both the sidecar (`signal_type=semantic`) and Tetragon (`signal_type=behavioral`).
- Route them to separate indices or datasets in your backend for correlation.

Verification:

```bash
# Use observability UI or direct queries against your backend to confirm semantic/behavioral spans are arriving
```

Expected: semantic events contain `identity`, `tool`, `riskTier` fields; behavioral events contain `pid`, `exec`/`connect` traces.

### 9) Simulate budget breach and verify automated response

Procedure:

1. Apply an `AIPolicy` with a low `semanticBudget.maxHighRiskActionsPerHour` (e.g., 1).
2. Trigger two high-risk calls from the sidecar (use integration test or curl into sidecar test endpoint).
3. Observe `SemanticBudget` status and `AIPolicy.Status.Quarantined` value.

Verification:

```bash
kubectl get semanticbudget -n ai-agents -o yaml
kubectl get aipolicy support-agent-policy -n ai-agents -o yaml
```

Expected: `SemanticBudget.status.currentHighRiskActions` increments and once threshold exceeded, `AIPolicy.status.quarantined` becomes `true` (if `quarantineOnBudgetExceeded` is set).

## Verification checklist (copyable)

- [ ] Images built and pushed: sidecar and operator
- [ ] Tetragon pods `Running` (if enabled)
- [ ] CRDs present: `AIPolicy`, `SemanticBudget`
- [ ] Operator process or Deployment running
- [ ] Webhook registered and TLS configured
- [ ] Example agent pod running with sidecar attached
- [ ] `AIPolicy` `Accepted=True` and `SemanticBudget` status present
- [ ] Sidecar health endpoint responding on each agent pod
- [ ] OPA tests for Rego policies passing
- [ ] Telemetry arriving in backend and correlated by `identity`
- [ ] Simulated budget breach results in quarantine when configured

## Troubleshooting tips

- If webhook calls are failing: inspect `ValidatingWebhookConfiguration` and the webhook server logs; confirm webhook TLS `CABundle` is correct.
- If Tetragon fails to initialize: check kernel logs (`dmesg`), `bpftool`, and ensure CNI is not blocking BPF maps.
- If sidecar cannot reach runtime aggregator: check Service/NetworkPolicy, `RUNTIME_AGGREGATOR_URL`, and that the operator's aggregator endpoint is listening (port :9443).

## Next steps

- Add runbook entries for alerts produced by semantic/behavioral correlation.
- CI jobs are now defined in `.github/workflows/ci.yml`, including `rego-test` and `kind-integration` for local kind-based sidecar validation.

---

For more details about architecture and design tradeoffs, see [docs/architecture.md](architecture.md).
