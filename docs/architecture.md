# Architecture

## Scope note

Everything in this document beyond standard Kubernetes primitives — the AI Workload Controller, `AIPolicy`, `SemanticBudget` — is a custom design this project implements, not an existing upstream Kubernetes or CNCF capability. The Operator is built with [Kubebuilder](https://kubebuilder.io); the CRDs are defined in `api/v1alpha1/` and only become real API resources once `config/crd/bases/` is applied to a cluster.

## The problem

Kubernetes security answers "where does this run and what does it consume." Agentic workloads need something that answers "what is this thing deciding to do." Those are different questions, and Kubernetes only has tooling for the first one.

A compromised agent pod doesn't look compromised. CPU, memory, restart count, error rate — all green, the whole time an attacker walks it through a privilege-escalation chain one plausible tool call at a time.

Three things compound this:

- **No intent signal.** Infra metrics don't capture "why did it call this tool." A hijacked agent is structurally indistinguishable from a busy one.
- **The LLM is a new attack surface.** An LLM in front of internal tools is an API with no fixed contract, reprogrammable at runtime by anyone who can type a prompt.
- **Infra controls answer the wrong question.** NetworkPolicy, PSA, quotas, RBAC gate existence and reachability. None of them evaluate whether a given decision matches the workload's intended purpose.

## Reference architecture

![Architecture diagram](architecture.png)

Request path: agent → sidecar → (if high-risk) operator → policy engine → tool.
Telemetry path: every hop feeds a behavioral pipeline, kept separate from infra metrics, that drives automated response.

## Design decisions

### 1. Tier the checks, don't run one check on everything

Routing every request through a full LLM classifier is 200ms+ and doesn't scale. Layer it instead:

| Approach | Added latency | Use it for |
|---|---|---|
| In-sidecar regex/heuristic filter | <1ms | Every request |
| Local quantized classifier, co-located | 2–15ms | Every request |
| Remote classifier over the network | 20–100ms+ | Never on the hot path — go async |
| Full LLM-as-judge | 200ms–2s | Async, or synchronous only for high-risk actions |

The real lever is risk-tiering the **action**, not the request. A read-only knowledge-base query gets waved through by tier 1/2. Anything that writes, sends, or calls out requires a synchronous check gated at tool invocation — because the prompt is usually benign-looking; it's the derived action that matters.

### 2. Sidecar enforces, operator decides

A sidecar only sees its own pod. It can't correlate across replicas, enforce a fleet-wide budget, block a bad manifest at admission, or persist trust scores past a pod's lifetime. That's the operator's job — a custom Kubernetes Operator (`controllers/`), not an off-the-shelf tool:

- **`AIPolicy`** (proposed CRD) — allowed tool scopes, risk thresholds, per identity.
- **`SemanticBudget`** (proposed CRD) — a custom resource inspired by `ResourceQuota`'s shape (a namespaced spending cap), but budgeting risk-weighted actions instead of CPU/memory. Kubernetes has no built-in concept of this; the operator defines and enforces it entirely on its own. This is what catches denial-of-wallet and slow-drip exfiltration — patterns invisible to any single sidecar.
- **Admission webhook** — part of the same operator; see the limitation below.
- **Runtime aggregator** — an HTTP endpoint (`controllers/runtime_aggregator.go`) that correlates verdict events per identity across the fleet and triggers quarantine.

**Rule of thumb:** sidecar enforces at the edge, operator owns policy and fleet correlation.

### 3. What the admission webhook can and can't do

An admission webhook only ever receives the standard admission payload — Deployment/Pod spec, annotations, labels, volumes, service account, secrets, image references, env vars. It has **no way to inspect** "allowed tools," "tool capabilities," "LLM plugins," or agent intent — none of that exists in the object Kubernetes hands it.

That's why `deploy/examples/agent-deployment.yaml` declares tool scope explicitly via annotations (`ai.security.internal/tools`, `ai.security.internal/policy`). The webhook (`webhook/v1alpha1/agent_webhook.go`) reads that annotation, fetches the referenced `AIPolicy`, and rejects the manifest if the declared scope exceeds it. **This is a declaration check, not behavioral introspection** — it cannot stop an agent from declaring a narrow scope and then calling something else at runtime. That gap is closed by the sidecar's real-time enforcement, not by the webhook.

### 4. eBPF gives kernel truth, not semantics

`deploy/tetragon/agent-egress-baseline.yaml` traces syscalls, egress connections, and exec events at the kernel boundary — independent of application self-reporting. It has **zero visibility** into the prompt, the model's reasoning, why a tool was selected, or intent of any kind; none of that exists at the syscall layer.

Semantic interpretation remains entirely in the user-space sidecar. eBPF's role is to provide trusted runtime evidence that correlates against the sidecar's semantic verdict — not to detect semantic attacks itself. "Semantic-behavioral telemetry" describes the *correlated output* of both signals, not either one in isolation; keep the two tagged separately at the source (`signal_type: semantic` vs. `signal_type: behavioral`) and join them downstream.

### 5. Give the SIEM a signal worth ingesting

Most SOC pipelines can't tell a busy agent from a compromised one because everything runs through the infra-metric lens. A denial-of-wallet attack and a legitimate traffic spike are the same shape on those axes. The fix is a second, structurally different signal — tool-call sequence entropy, deviation from declared purpose, cross-session correlation — on its own pipeline, with its own baselines, not a new panel on the existing dashboard.

### 6. Policy-as-code should consume risk scores, not replace them

`policies/rego/ai_policy.rego` keys decisions off `risk_score` and `identity_quarantined`, fields populated upstream by the sidecar classifier. The policy evaluation itself stays deterministic and unit-testable (`policies/rego/ai_policy_test.rego`); all the non-determinism lives in the classifier that produces the score.

## Component responsibility summary

| Component | Can see | Can't see | Enforces |
|---|---|---|---|
| Sidecar | Its own pod's prompts and tool calls | Other pods, fleet-wide patterns | Tier-1/2 filtering, per-request |
| Admission webhook | Declared annotations vs. `AIPolicy` | Runtime behavior, actual tool calls | Declaration matches policy, at schedule time |
| Operator / runtime aggregator | Correlated verdicts across the fleet | Individual prompt content | `SemanticBudget`, quarantine |
| Tetragon (eBPF) | Syscalls, egress, exec | Prompts, reasoning, intent | Kernel-truth evidence for correlation |
