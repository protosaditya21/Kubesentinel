package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SemanticBudgetSpecStandalone is the standalone form of a risk budget, for
// cases where you want to budget a namespace or a group of identities rather
// than embedding the budget in a single AIPolicy. Conceptually this mirrors
// ResourceQuota's shape (a namespaced spending cap) but nothing here is a
// built-in Kubernetes concept — the operator defines, tracks, and enforces
// this entirely on its own from events it receives from the sidecars.
type SemanticBudgetSpecStandalone struct {
	// Scope selects which identities this budget applies to. Use either
	// Identity for a single agent or Selector to cover a group.
	// +optional
	Identity string `json:"identity,omitempty"`

	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// MaxHighRiskActionsPerHour is the aggregate cap across every identity
	// covered by this budget.
	// +kubebuilder:validation:Minimum=0
	MaxHighRiskActionsPerHour int32 `json:"maxHighRiskActionsPerHour"`

	// MaxTotalActionsPerHour optionally caps total tool-call volume
	// regardless of risk tier — this is the knob that catches denial-of-wallet
	// patterns where every individual call looks low-risk.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxTotalActionsPerHour *int32 `json:"maxTotalActionsPerHour,omitempty"`

	// QuarantineOnBudgetExceeded tells the runtime aggregator to quarantine
	// affected identities rather than just logging the breach.
	// +optional
	QuarantineOnBudgetExceeded bool `json:"quarantineOnBudgetExceeded,omitempty"`
}

// SemanticBudgetStatus reports current usage against the budget.
type SemanticBudgetStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentHighRiskActions is the count observed in the active window.
	// +optional
	CurrentHighRiskActions int32 `json:"currentHighRiskActions,omitempty"`

	// WindowStart marks when the current counting window began.
	// +optional
	WindowStart *metav1.Time `json:"windowStart,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MaxHighRisk/hr",type=integer,JSONPath=`.spec.maxHighRiskActionsPerHour`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentHighRiskActions`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SemanticBudget is a custom resource, defined by this project, that caps
// risk-weighted agent actions the way ResourceQuota caps CPU/memory — but
// this is not a Kubernetes built-in. It's an independent CRD this operator
// defines, reconciles, and enforces using events reported by the sidecars,
// with no equivalent upstream capability to fall back on.
type SemanticBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SemanticBudgetSpecStandalone `json:"spec,omitempty"`
	Status SemanticBudgetStatus         `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SemanticBudgetList contains a list of SemanticBudget.
type SemanticBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SemanticBudget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SemanticBudget{}, &SemanticBudgetList{})
}
