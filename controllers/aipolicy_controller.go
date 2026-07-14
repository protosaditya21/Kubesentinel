package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/your-org/ai-workload-security/api/v1alpha1"
)

// AIPolicyReconciler reconciles an AIPolicy object.
//
// Scope: this reconciler validates the policy shape and sets the Accepted
// condition. It does NOT inspect what the agent workload actually does —
// admission-time enforcement lives in webhook/v1alpha1, and runtime budget
// enforcement lives in RuntimeAggregator. This reconciler's only job is to
// keep AIPolicy.Status accurate so both of those can trust it.
type AIPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=security.internal,resources=aipolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.internal,resources=aipolicies/status,verbs=get;update;patch

func (r *AIPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var policy securityv1alpha1.AIPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cond := metav1.Condition{
		Type:               "Accepted",
		Status:             metav1.ConditionTrue,
		Reason:             "Validated",
		Message:            fmt.Sprintf("policy accepted for identity %q with %d allowed tools", policy.Spec.Identity, len(policy.Spec.AllowedTools)),
		ObservedGeneration: policy.Generation,
	}

	if policy.Spec.Identity == "" {
		cond = metav1.Condition{
			Type:               "Accepted",
			Status:             metav1.ConditionFalse,
			Reason:             "MissingIdentity",
			Message:            "spec.identity must be set",
			ObservedGeneration: policy.Generation,
		}
	}

	meta.SetStatusCondition(&policy.Status.Conditions, cond)

	if err := r.Status().Update(ctx, &policy); err != nil {
		logger.Error(err, "unable to update AIPolicy status")
		return ctrl.Result{}, err
	}

	logger.Info("reconciled AIPolicy", "identity", policy.Spec.Identity, "accepted", cond.Status == metav1.ConditionTrue)
	return ctrl.Result{}, nil
}

func (r *AIPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.AIPolicy{}).
		Complete(r)
}
