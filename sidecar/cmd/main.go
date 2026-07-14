// Command sidecar implements the semantic guardrail sidecar described in
// the architecture doc (\u00A76.1). It sits in front of an agent's outbound
// tool calls: tier-1 regex filtering happens in-process, tier-2 defers to
// a pluggable Classifier, and every high-risk verdict gets reported to the
// operator's runtime aggregator endpoint so budget enforcement (\u00A74.2) works
// across the whole fleet, not just this one pod.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/your-org/ai-workload-security/sidecar/filters"
)

type checkRequest struct {
	Identity  string `json:"identity"`
	Tool      string `json:"tool"`
	RiskTier  string `json:"riskTier"`
	Prompt    string `json:"prompt"`
	Namespace string `json:"namespace"`
	Budget    string `json:"budgetName"`
}

type checkResponse struct {
	Allowed bool    `json:"allowed"`
	Reason  string  `json:"reason,omitempty"`
	Score   float64 `json:"score"`
}

func main() {
	regexFilter, err := filters.NewRegexFilter(filters.DefaultPatterns())
	if err != nil {
		log.Fatalf("failed to compile default patterns: %v", err)
	}

	classifier := filters.Classifier(filters.NoopClassifier{})
	aggregatorURL := os.Getenv("RUNTIME_AGGREGATOR_URL") // e.g. http://ai-workload-controller.ai-security:9443/v1/verdicts

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","classifier":"loaded"}`))
	})

	mux.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		var req checkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Tier 1.
		v := regexFilter.Check(req.Prompt)
		if v.Blocked {
			respond(w, checkResponse{Allowed: false, Reason: v.Reason, Score: v.Score})
			reportVerdict(aggregatorURL, req, v)
			return
		}

		// Tier 2.
		v2, err := classifier.Classify(r.Context(), req.Prompt)
		if err != nil {
			http.Error(w, "classifier error", http.StatusInternalServerError)
			return
		}

		respond(w, checkResponse{Allowed: !v2.Blocked, Reason: v2.Reason, Score: v2.Score})

		if req.RiskTier == "high" {
			reportVerdict(aggregatorURL, req, v2)
		}
	})

	log.Println("semantic guardrail sidecar listening on :9090")
	log.Fatal(http.ListenAndServe(":9090", mux))
}

func respond(w http.ResponseWriter, resp checkResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// reportVerdict fires-and-forgets a verdict event to the operator's runtime
// aggregator so fleet-wide SemanticBudget tracking stays accurate. Failures
// here are logged, not fatal — losing one telemetry event shouldn't take
// down the request path.
func reportVerdict(aggregatorURL string, req checkRequest, v filters.Verdict) {
	if aggregatorURL == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"identity":   req.Identity,
		"namespace":  req.Namespace,
		"tool":       req.Tool,
		"riskTier":   req.RiskTier,
		"riskScore":  v.Score,
		"allowed":    !v.Blocked,
		"budgetName": req.Budget,
	})
	go func() {
		resp, err := http.Post(aggregatorURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			log.Printf("failed to report verdict to aggregator: %v", err)
			return
		}
		defer resp.Body.Close()
	}()
}
