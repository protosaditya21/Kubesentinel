package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/your-org/ai-workload-security/api/v1alpha1"
)

// RuntimeAggregator exposes an HTTP endpoint that sidecars POST verdict
// events to. This is the piece that gives the operator fleet-wide visibility
// a single sidecar can never have on its own: it correlates events by
// identity across every replica and every namespace it's watching, and
// updates SemanticBudget.Status accordingly.
//
// This is intentionally a thin HTTP handler, not a full event-streaming
// system — for production scale, swap this for a proper queue (NATS,
// Kafka, etc.) feeding the same UpdateBudget logic.
type RuntimeAggregator struct {
	client.Client
}

// VerdictEvent is what a sidecar reports after a tool-call decision.
type VerdictEvent struct {
	Identity   string  `json:"identity"`
	Namespace  string  `json:"namespace"`
	Tool       string  `json:"tool"`
	RiskTier   string  `json:"riskTier"`
	RiskScore  float64 `json:"riskScore"`
	Allowed    bool    `json:"allowed"`
	BudgetName string  `json:"budgetName"`
}

func (a *RuntimeAggregator) HandleVerdict(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	var ev VerdictEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "invalid verdict payload", http.StatusBadRequest)
		return
	}

	if ev.RiskTier != "high" {
		// Only high-risk actions count against the semantic budget by design —
		// see docs/architecture.md \u00A74.1 on risk-tiering.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if err := a.incrementBudget(ctx, ev); err != nil {
		logger.Error(err, "failed to update SemanticBudget", "identity", ev.Identity)
		http.Error(w, "failed to update budget", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (a *RuntimeAggregator) incrementBudget(ctx context.Context, ev VerdictEvent) error {
	var budget securityv1alpha1.SemanticBudget
	key := client.ObjectKey{Namespace: ev.Namespace, Name: ev.BudgetName}
	if err := a.Get(ctx, key, &budget); err != nil {
		return err
	}

	budget.Status.CurrentHighRiskActions++
	return a.Status().Update(ctx, &budget)
}
