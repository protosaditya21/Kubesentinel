package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RiskTier classifies how much scrutiny a tool call requires before it's allowed to run.
type RiskTier string

const (
	RiskTierLow    RiskTier = "low"
	RiskTierMedium RiskTier = "medium"
	RiskTierHigh   RiskTier = "high"
)

// AllowedTool declares one tool an agent identity is permitted to call, and the
// risk handling that applies to it. This is declared metadata, not an inferred
// capability — the operator has no way to discover what tools an agent actually
// has short of what's declared here and cross-checked at admission time.
type AllowedTool struct {
	// Name of the tool, matching the identifier used by the sidecar/agent.
	Name string `json:"name"`

	// RiskTier controls whether calls to this tool require a synchronous
	// semantic check before being allowed through.
	// +kubebuilder:validation:Enum=low;medium;high
	RiskTier RiskTier `json:"riskTier"`

	// RequiresSyncCheck forces a synchronous tier-2/3 semantic check on every
	// call to this tool, regardless of the sidecar's cached verdict.
	// +optional
	RequiresSyncCheck bool `json:"requiresSyncCheck,omitempty"`

	// MaxCallsPerHour caps how often this identity may call this specific tool.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxCallsPerHour *int32 `json:"maxCallsPerHour,omitempty"`
}

// SemanticBudgetSpec is the embedded risk budget for an identity. It's also
// exposed standalone as the SemanticBudget CRD (semanticbudget_types.go) for
// cases where the budget is managed independently of a specific AIPolicy.
type SemanticBudgetSpec struct {
	// MaxHighRiskActionsPerHour caps total high-risk tool calls across all
	// tools for this identity, independent of the per-tool caps above.
	// +kubebuilder:validation:Minimum=0
	MaxHighRiskActionsPerHour int32 `json:"maxHighRiskActionsPerHour"`

	// QuarantineOnBudgetExceeded, if true, tells the runtime aggregator to
	// quarantine the identity (see AIPolicyStatus.Quarantined) rather than
	// just logging the breach.
	// +optional
	QuarantineOnBudgetExceeded bool `json:"quarantineOnBudgetExceeded,omitempty"`
}

// AIPolicySpec defines the tool scope and risk budget for one agent identity.
type AIPolicySpec struct {
	// Identity is the agent identity this policy applies to. Must match the
	// identity the sidecar and admission webhook use to look up this policy
	// (typically the ServiceAccount name or an explicit annotation value).
	Identity string `json:"identity"`

	// AllowedTools is the declared, admission-checkable tool scope for this identity.
	AllowedTools []AllowedTool `json:"allowedTools,omitempty"`

	// SemanticBudget is this identity's risk budget.
	// +optional
	SemanticBudget *SemanticBudgetSpec `json:"semanticBudget,omitempty"`
}

// AIPolicyStatus reflects what the controller has observed and decided about this identity.
type AIPolicyStatus struct {
	// Conditions follow the standard Kubernetes condition pattern
	// (e.g. type=Accepted, status=True/False).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Quarantined is set true by the runtime aggregator when this identity's
	// SemanticBudget has been exceeded and QuarantineOnBudgetExceeded is set.
	// +optional
	Quarantined bool `json:"quarantined,omitempty"`

	// HighRiskActionsInWindow is the current count of high-risk actions
	// observed for this identity in the active budget window.
	// +optional
	HighRiskActionsInWindow int32 `json:"highRiskActionsInWindow,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Identity",type=string,JSONPath=`.spec.identity`
// +kubebuilder:printcolumn:name="Quarantined",type=boolean,JSONPath=`.status.quarantined`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIPolicy is a custom resource, defined by this project, that declares the
// allowed tool scope and risk budget for one agentic workload identity.
// It is not an upstream Kubernetes resource; it only exists once this
// operator's CRD is installed (config/crd/bases/security.internal_aipolicies.yaml).
type AIPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIPolicySpec   `json:"spec,omitempty"`
	Status AIPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIPolicyList contains a list of AIPolicy.
type AIPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIPolicy{}, &AIPolicyList{})
}
