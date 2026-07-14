// Package v1alpha1 implements the ValidatingWebhook for agent Deployments.
//
// IMPORTANT SCOPE NOTE: an admission webhook only ever receives the
// standard admission payload — the Deployment/Pod spec, annotations,
// labels, volumes, service account, secrets, image references, env vars.
// It has no way to inspect "allowed tools," "tool capabilities," "LLM
// plugins," or agent intent — none of that exists in the object Kubernetes
// hands it. This handler can only check what the workload has explicitly
// declared via the ai.security.internal/tools and ai.security.internal/policy
// annotations (see deploy/examples/agent-deployment.yaml) against the
// referenced AIPolicy. It is a declaration check, not behavioral
// introspection: it cannot stop an agent from declaring a narrow scope and
// then calling something else at runtime. That gap is closed by the
// sidecar's real-time enforcement, not by this webhook.
package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	securityv1alpha1 "github.com/your-org/ai-workload-security/api/v1alpha1"
)

const (
	annotationPolicy = "ai.security.internal/policy"
	annotationTools  = "ai.security.internal/tools"
)

// AgentDeploymentValidator validates that a Deployment's declared tool scope
// (via annotations) doesn't exceed the AIPolicy it references.
type AgentDeploymentValidator struct {
	Client  client.Client
	Decoder admission.Decoder
}

func (v *AgentDeploymentValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx)

	var deploy appsv1.Deployment
	if err := v.Decoder.Decode(req, &deploy); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	toolsAnnotation, hasTools := deploy.Annotations[annotationTools]
	policyName, hasPolicy := deploy.Annotations[annotationPolicy]

	if !hasTools || !hasPolicy {
		// No declaration at all: nothing to validate against. Whether that's
		// allowed or rejected is a policy choice for your cluster — default
		// here is to allow, on the assumption non-agent workloads shouldn't
		// be forced to carry these annotations. Tighten with a namespace
		// selector in config/webhook if you want to require declaration.
		return admission.Allowed("no AI tool-scope annotations present; skipping semantic admission check")
	}

	var policy securityv1alpha1.AIPolicy
	key := client.ObjectKey{Namespace: deploy.Namespace, Name: policyName}
	if err := v.Client.Get(ctx, key, &policy); err != nil {
		return admission.Denied(fmt.Sprintf("referenced AIPolicy %q not found in namespace %q: %v", policyName, deploy.Namespace, err))
	}

	if policy.Status.Quarantined {
		return admission.Denied(fmt.Sprintf("identity %q is currently quarantined by SemanticBudget enforcement", policy.Spec.Identity))
	}

	declaredTools := splitAndTrim(toolsAnnotation)
	allowed := make(map[string]bool, len(policy.Spec.AllowedTools))
	for _, t := range policy.Spec.AllowedTools {
		allowed[t.Name] = true
	}

	var violations []string
	for _, tool := range declaredTools {
		if !allowed[tool] {
			violations = append(violations, tool)
		}
	}

	if len(violations) > 0 {
		logger.Info("rejecting deployment: declared tool scope exceeds AIPolicy",
			"deployment", deploy.Name, "namespace", deploy.Namespace, "violations", violations)
		return admission.Denied(fmt.Sprintf(
			"declared tools %v are not present in AIPolicy %q's allowedTools; this only checks the declaration, not runtime behavior",
			violations, policyName))
	}

	return admission.Allowed("declared tool scope is within AIPolicy limits")
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Ensure the response body serializes as expected by the admission machinery
// even in non-controller-runtime test harnesses.
var _ = json.Marshal
var _ = admissionv1.AdmissionReview{}
