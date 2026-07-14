# AI Workload Security for Kubernetes

Kubernetes secures where code runs and what it consumes. This project is a reference implementation for defending agentic LLM workloads by combining semantic-sidecar filtering, cluster-wide policy via a Kubernetes Operator, and kernel-level observability using Tetragon/eBPF.

**Status:** early-stage reference implementation, not production-hardened. Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Summary

This repo provides:

- A sidecar component (low-latency filtering + pluggable classifier).
- A Kubernetes Operator that defines two CRDs: `AIPolicy` and `SemanticBudget`.
- An admission webhook that validates agent manifests against declared `AIPolicy` rules.
- A runtime aggregator endpoint that the sidecar uses to report high-risk verdicts, which the operator uses for budget/quarantine decisions.
- Optional Tetragon/eBPF tracing to correlate kernel-level events with semantic verdicts.

Read the design rationale and boundaries in [docs/architecture.md](docs/architecture.md).

## Components (what to look at)

- **Sidecar**: `sidecar/` — tier-1 (regex) and tier-2 (classifier interface) filtering. Reports high-risk calls to the operator's runtime endpoint.
- **API types**: `api/v1alpha1/` — Go types for `AIPolicy` and `SemanticBudget` (see `api/v1alpha1/*.go`).
- **Controllers**: `controllers/` — `AIPolicyReconciler`, `SemanticBudgetReconciler`, and `RuntimeAggregator` (see `controllers/*.go`).
- **Webhook**: `webhook/v1alpha1/` — admission-time validation of agent manifests against `AIPolicy` constraints.
- **Tetragon**: `deploy/tetragon/` — example eBPF tracing policies and baseline manifests for kernel-level evidence.
- **Rego**: `policies/rego/` — example deterministic policy rules that consume the risk signal.

## Quick start (developer/testing)

Prerequisites: Kubernetes 1.27+, Helm 3, Docker/CRI to build images.

Build and deploy operator locally (minikube/kind):

```bash
# Build sidecar (optional if using published images)
cd sidecar && docker build -t <registry>/semantic-guardrail:dev .

# Build operator image
cd .. && docker build -t <registry>/ai-workload-controller:dev .

# Apply CRDs and RBAC
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/role.yaml

# Install chart (or run controller locally via 'make run')
helm install ai-workload-controller charts/ai-workload-controller -n ai-security --create-namespace
```

Deploy an example agent and policy:

```bash
kubectl apply -f deploy/examples/agent-deployment.yaml
kubectl apply -f config/samples/aipolicy_sample.yaml
kubectl label namespace ai-agents ai-workload=true
kubectl apply -f config/webhook/manifests.yaml
```

See [docs/deployment-guide.md](docs/deployment-guide.md) for verification steps (health/readiness, webhook behavior, and budget simulation).

## CRD Reference (high level)

- `AIPolicy` (api/v1alpha1):
	- `spec.identity` (string): Agent identity this policy applies to. Required.
	- `spec.allowedTools` (list): Tools the agent is allowed to call.
	- `status.conditions`: `Accepted` condition is set by the controller; `status.quarantined` is toggled by budget enforcement.

- `SemanticBudget` (api/v1alpha1):
	- `spec.identity` (string): Identity or selector scope.
	- `spec.maxHighRiskActionsPerHour` (int): Budget threshold.
	- `spec.quarantineOnBudgetExceeded` (bool): Whether to flip quarantine flag when exceeded.
	- `status.WindowStart` / `status.CurrentHighRiskActions`: Managed by the controller and runtime aggregator.

Controller behavior (see `controllers/`):

- `AIPolicyReconciler` validates policy shape and sets the `Accepted` condition; it does not enforce runtime behavior directly (`webhook/` and sidecar do that).
- `SemanticBudgetReconciler` manages hourly windows and triggers quarantine actions when budgets are exceeded.
- Runtime verdict ingestion is exposed at `/v1/verdicts` by the runtime aggregator (see `controllers/runtime_aggregator.go`).

## Example CRs

Example `AIPolicy` for a support agent (see `config/samples/aipolicy_sample.yaml`):

```yaml
apiVersion: security.internal/v1alpha1
kind: AIPolicy
metadata:
	name: support-agent-policy
	namespace: ai-agents
spec:
	identity: support-agent
	allowedTools:
		- name: kb-search
			riskTier: low
		- name: ticket-update
			riskTier: medium
			requiresSyncCheck: true
		- name: billing-refund
			riskTier: high
			requiresSyncCheck: true
			maxCallsPerHour: 5
	semanticBudget:
		maxHighRiskActionsPerHour: 10
		quarantineOnBudgetExceeded: true
```

Example standalone `SemanticBudget` (see `config/samples/semanticbudget_sample.yaml`):

```yaml
apiVersion: security.internal/v1alpha1
kind: SemanticBudget
metadata:
	name: support-agent-budget
	namespace: ai-agents
spec:
	identity: support-agent
	maxHighRiskActionsPerHour: 10
	maxTotalActionsPerHour: 200
	quarantineOnBudgetExceeded: true
```

## Development notes

- Run the controller locally with debugging flags: `go run ./main.go --metrics-bind-address=:8080 --health-probe-bind-address=:8081`.
- The webhook path is registered at `/validate-agent-manifest` when `--enable-webhook` is true (default).
- The runtime aggregator listens on `:9443` (in-tree) for verdict POSTs from sidecars.

## Files of interest

- `main.go` — manager setup, controller registration, webhook registration.
- `controllers/aipolicy_controller.go` — policy validation and status updates.
- `controllers/semanticbudget_controller.go` — budget lifecycle and quarantine logic.
- `config/samples/aipolicy_sample.yaml` — example policy.
- `deploy/examples/agent-deployment.yaml` — example agent + sidecar wiring.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines and development workflow.

## License

Apache 2.0 — see [LICENSE](LICENSE).
