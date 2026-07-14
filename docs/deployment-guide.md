# Deployment Guide

Prerequisites:

| Requirement | Notes |
|---|---|
| Kubernetes cluster | v1.27+, cluster-admin for CRDs and webhooks |
| Cilium CNI | Needed for Tetragon. Different CNI? Run Tetragon standalone. |
| Policy engine | OPA Gatekeeper or Kyverno already enforcing on target namespaces |
| Helm 3 | For Tetragon and the operator chart |
| Registry access | Somewhere to push the sidecar and operator images |
| Telemetry backend | An OTel collector + a SIEM that can host a separate semantic-behavioral index |

## 1. Sidecar

```bash
cd sidecar
go build ./...              # sanity check
docker build -t <registry>/semantic-guardrail:latest .
docker push <registry>/semantic-guardrail:latest
```

Reference `deploy/examples/agent-deployment.yaml` for how to wire the sidecar container, the `ai.security.internal/tools` / `ai.security.internal/policy` annotations, and the `RUNTIME_AGGREGATOR_URL` env var into your agent's Deployment.

**Verify:**
```bash
kubectl exec -n ai-agents deploy/agent-workload -c agent -- curl -s localhost:9090/healthz
```
Expect: `{"status":"ok","classifier":"loaded"}`

## 2. Tetragon (eBPF)

```bash
helm repo add cilium https://helm.cilium.io
helm install tetragon cilium/tetragon -n kube-system
kubectl apply -f deploy/tetragon/agent-egress-baseline.yaml
```

**Verify:**
```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=tetragon
kubectl logs -n kube-system -l app.kubernetes.io/name=tetragon -c export-stdout --tail=20
```
Expect: pods `Running` on every node, `tcp_connect` events streaming for `workload-type: ai-agent`.

## 3. Operator (AIPolicy / SemanticBudget CRDs, controllers, webhook)

```bash
docker build -t <registry>/ai-workload-controller:latest .
docker push <registry>/ai-workload-controller:latest

kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/role.yaml
helm install ai-workload-controller charts/ai-workload-controller \
  -n ai-security --create-namespace \
  --set image.repository=<registry>/ai-workload-controller

kubectl apply -f config/samples/aipolicy_sample.yaml
```

**Verify:**
```bash
kubectl get aipolicy -n ai-agents
kubectl describe aipolicy support-agent-policy -n ai-agents
```
Expect: `Accepted: True` condition set.

## 4. Admission webhook

```bash
kubectl label namespace ai-agents ai-workload=true
kubectl apply -f config/webhook/manifests.yaml
```

**Verify (should be rejected):**
```bash
kubectl apply -f deploy/examples/overprivileged-agent.yaml
```
Expect: denied — the manifest's `ai.security.internal/tools` annotation lists `wire-transfer`, which isn't in `support-agent-policy`'s `allowedTools`.

## 5. Policy-as-code

```bash
opa test policies/rego -v
```
All four bundled tests should pass. Extend `ai_policy.rego` with your own risk thresholds and add corresponding cases to `ai_policy_test.rego`.

## 6. Telemetry pipeline

Point your OTel collector at both the sidecar's `signal_type: semantic` spans and Tetragon's `signal_type: behavioral` spans, routed to an index separate from your infra Prometheus/Grafana stack. Correlate by identity and time window before forwarding high-risk events to your SIEM.

## 7. Automated response

The `SemanticBudgetReconciler` (`controllers/semanticbudget_controller.go`) already quarantines the matching `AIPolicy` when `quarantineOnBudgetExceeded` is set and the budget is breached — wire your sidecar and webhook to check `AIPolicy.Status.Quarantined` (the webhook already does) and fall back to a restricted tool set or reject calls outright.

## Rollout checklist

- [ ] Sidecar `/healthz` responds on every agent pod; tool calls visibly routing through `localhost:9090`
- [ ] Tetragon `Running` on all nodes; `tcp_connect`/`execve` events visible for agent-labeled pods
- [ ] `AIPolicy` + `SemanticBudget` applied for every agent identity in scope
- [ ] Admission webhook rejects `deploy/examples/overprivileged-agent.yaml`
- [ ] `opa test policies/rego` green
- [ ] Semantic and behavioral telemetry landing in their own SIEM index, not mixed with infra metrics
- [ ] Simulated budget breach triggers quarantine within the expected window
- [ ] On-call runbook updated with the new alert types and response actions
