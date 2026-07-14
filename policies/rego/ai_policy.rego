# Policy-as-code extended to consume semantic risk scores as first-class
# inputs, per docs/architecture.md \u00A76.5. The policy evaluation itself stays
# deterministic — all non-determinism lives in the classifier that produces
# risk_score/intent_category upstream of this policy.
package ai.toolcall

import future.keywords.in

default allow := false

# Low-risk tools: always allow, unless the identity is quarantined.
allow if {
	input.tool_risk_tier == "low"
	not input.identity_quarantined
}

# Medium/high-risk tools: require a risk score under threshold AND that the
# identity isn't currently quarantined.
allow if {
	input.tool_risk_tier in {"medium", "high"}
	input.risk_score < threshold_for(input.tool_risk_tier)
	not input.identity_quarantined
}

threshold_for(tier) := 0.5 if tier == "medium"
threshold_for(tier) := 0.2 if tier == "high"

deny_reason contains msg if {
	not allow
	input.identity_quarantined
	msg := sprintf("identity %v is quarantined", [input.identity])
}

deny_reason contains msg if {
	not allow
	not input.identity_quarantined
	msg := sprintf("risk_score %v exceeds threshold for tier %v", [input.risk_score, input.tool_risk_tier])
}
