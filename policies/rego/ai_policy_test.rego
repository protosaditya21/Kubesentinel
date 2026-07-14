package ai.toolcall

import future.keywords.in

test_low_risk_always_allowed if {
	allow with input as {"tool_risk_tier": "low", "risk_score": 0.99, "identity_quarantined": false}
}

test_high_risk_low_score_allowed if {
	allow with input as {"tool_risk_tier": "high", "risk_score": 0.05, "identity_quarantined": false}
}

test_high_risk_high_score_denied if {
	not allow with input as {"tool_risk_tier": "high", "risk_score": 0.8, "identity_quarantined": false}
}

test_quarantined_identity_denied if {
	not allow with input as {"tool_risk_tier": "low", "risk_score": 0.0, "identity_quarantined": true}
}
