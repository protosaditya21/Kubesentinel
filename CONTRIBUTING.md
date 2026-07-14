# Contributing

Thanks for considering a contribution. This is an early-stage reference implementation — there's a lot of room for real work here.

## Getting started

```bash
git clone <your fork>
cd ai-workload-security
go build ./...            # operator
cd sidecar && go build ./...
opa test policies/rego -v
helm lint charts/ai-workload-controller
```

## Where to start

- **Sidecar classifier**: `sidecar/filters/classifier.go` ships a `NoopClassifier`. A real tier-2 implementation (distilled model, ONNX runtime, whatever) is one of the highest-value contributions available.
- **Operator tests**: `controllers/` has no test suite yet. `envtest`-based reconciler tests would be very welcome.
- **Rego policies**: `policies/rego/ai_policy.rego` is a starting point, not a complete policy set.
- **Docs**: `docs/architecture.md` documents design intent; if implementation and docs drift, both need fixing together.

## Ground rules

- Every PR that touches `api/v1alpha1/` should also update the matching CRD YAML in `config/crd/bases/` — they're maintained by hand in this repo (no `controller-gen` run in CI yet; happy to take a PR that adds it).
- Don't claim a component sees more than it does. If you're adding a check, be explicit in code comments about what data it actually has access to versus what it's inferring — see `webhook/v1alpha1/agent_webhook.go` for the pattern this project follows.
- Keep policy evaluation deterministic. Non-determinism belongs in classifiers, not in the Rego/reconciler logic that consumes their output.

## Submitting a PR

1. Fork, branch, commit with a clear message.
2. Fill out the PR template — which component(s), how it was tested.
3. CI runs `go build`/`go vet` for the operator and sidecar, `opa test`, and `helm lint`. All should pass.
4. One maintainer approval to merge.

## Reporting bugs / requesting features

Use the issue templates under `.github/ISSUE_TEMPLATE/`.
