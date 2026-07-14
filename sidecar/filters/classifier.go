package filters

import "context"

// Classifier is the tier-2 interface: a small, purpose-built model (not the
// production LLM) scoring injection likelihood or intent category. Ship your
// own implementation — gRPC to a co-located model server, an ONNX runtime
// call, whatever fits — and wire it in via cmd/sidecar/main.go. Keep it off
// the network hot path: co-locate it or talk to it over a local socket, per
// \u00A74.1 of the architecture doc.
type Classifier interface {
	Classify(ctx context.Context, prompt string) (Verdict, error)
}

// NoopClassifier always allows. Useful for local dev / testing the sidecar's
// plumbing without standing up a real model.
type NoopClassifier struct{}

func (NoopClassifier) Classify(_ context.Context, _ string) (Verdict, error) {
	return Verdict{Blocked: false, Score: 0.0}, nil
}
