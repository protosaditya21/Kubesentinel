# Security Policy

## Reporting a vulnerability

Please do not open a public issue for suspected security vulnerabilities.
Instead, email the maintainers (see repository metadata for current contact)
with:

- A description of the vulnerability and its potential impact
- Steps to reproduce
- Any relevant logs or PoC code

We'll acknowledge receipt within 5 business days.

## Scope

This project is a reference implementation for semantic security controls on
Kubernetes. Note the explicit limitations documented in `docs/architecture.md`:
the admission webhook validates *declared* metadata, not runtime behavior, and
eBPF tracing provides kernel-level evidence, not semantic interpretation.
Vulnerability reports that stem from these documented, by-design limitations
should still be reported if you believe they're exploitable in a way not
already covered by the docs — but please read `docs/architecture.md` first.
