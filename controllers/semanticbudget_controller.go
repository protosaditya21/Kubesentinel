package controllers

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/your-org/ai-workload-security/api/v1alpha1"
)

// SemanticBudgetReconciler reconciles a SemanticBudget object.
//
// It owns the counting-window lifecycle (rolling the window over once an
// hour) and quarantine decisions. Actual event ingestion — the sidecar
// telling us "identity X made a high-risk call" — happens over the
// RuntimeAggregator's event endpoint (runtime_aggregator.go), which updates
// SemanticBudget.Status directly. This reconciler mostly reacts to those
// status changes and resets the window.
type SemanticBudgetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=security.internal,resources=semanticbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.internal,resources=semanticbudgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.internal,resources=aipolicies,verbs=get;list;watch;update;patch

const budgetWindow = 1 * time.Hour

func (r *SemanticBudgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var budget securityv1alpha1.SemanticBudget
	if err := r.Get(ctx, req.NamespacedName, &budget); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := validateBudgetScope(&budget); err != nil {
		logger.Error(err, "invalid semantic budget scope", "namespace", budget.Namespace, "name", budget.Name)
		return ctrl.Result{}, err
	}

	now := metav1.Now()
	if budget.Status.WindowStart == nil || now.Sub(budget.Status.WindowStart.Time) > budgetWindow {
		logger.Info("rolling over budget window", "name", budget.Name)
		budget.Status.WindowStart = &now
		budget.Status.CurrentHighRiskActions = 0
		if err := r.Status().Update(ctx, &budget); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: budgetWindow}, nil
	}

	if budget.Status.CurrentHighRiskActions >= budget.Spec.MaxHighRiskActionsPerHour {
		logger.Info("semantic budget exceeded", "name", budget.Name,
			"current", budget.Status.CurrentHighRiskActions, "max", budget.Spec.MaxHighRiskActionsPerHour)
		if budget.Spec.QuarantineOnBudgetExceeded {
			if err := r.quarantineIdentity(ctx, budget); err != nil {
				logger.Error(err, "failed to quarantine identity after budget breach")
				return ctrl.Result{}, err
			}
		}
	}

	// Re-check again before the window naturally rolls over.
	remaining := budgetWindow - now.Sub(budget.Status.WindowStart.Time)
	return ctrl.Result{RequeueAfter: remaining}, nil
}

func validateBudgetScope(budget *securityv1alpha1.SemanticBudget) error {
	if budget == nil {
		return fmt.Errorf("semantic budget is nil")
	}

	if budget.Spec.Identity == "" && budget.Spec.Selector == nil {
		return fmt.Errorf("semantic budget %s/%s must set either spec.identity or spec.selector", budget.Namespace, budget.Name)
	}
	if budget.Spec.Identity != "" && budget.Spec.Selector != nil {
		return fmt.Errorf("semantic budget %s/%s cannot set both spec.identity and spec.selector", budget.Namespace, budget.Name)
	}
	if budget.Spec.Selector != nil {
		if _, err := metav1.LabelSelectorAsSelector(budget.Spec.Selector); err != nil {
			return fmt.Errorf("semantic budget %s/%s has an invalid selector: %w", budget.Namespace, budget.Name, err)
		}
	}
	return nil
}

// quarantineIdentity flips the matching AIPolicy's Quarantined status flag.
// This is what §6.7 in the design doc calls "automated response" — the
// sidecar and admission webhook are expected to check this flag and fall
// back to a restricted tool set (or reject calls outright) once it's set.
func (r *SemanticBudgetReconciler) quarantineIdentity(ctx context.Context, budget securityv1alpha1.SemanticBudget) error {
	if err := validateBudgetScope(&budget); err != nil {
		return err
	}

	var policies securityv1alpha1.AIPolicyList
	if err := r.List(ctx, &policies, client.InNamespace(budget.Namespace)); err != nil {
		return err
	}

	selector, err := selectorForBudget(budget)
	if err != nil {
		return err
	}

	var updatedAny bool
	for i := range policies.Items {
		p := &policies.Items[i]
		if budget.Spec.Identity != "" {
			if p.Spec.Identity != budget.Spec.Identity {
				continue
			}
		} else if selector != nil && !selector.Matches(labels.Set(p.Labels)) {
			continue
		}

		if p.Status.Quarantined {
			continue
		}

		p.Status.Quarantined = true
		if err := r.Status().Update(ctx, p); err != nil {
			return err
		}
		updatedAny = true
	}

	if !updatedAny {
		return nil
	}
	return nil
}

func selectorForBudget(budget securityv1alpha1.SemanticBudget) (labels.Selector, error) {
	if budget.Spec.Identity != "" {
		return nil, nil
	}
	if budget.Spec.Selector == nil {
		return nil, nil
	}
	return metav1.LabelSelectorAsSelector(budget.Spec.Selector)
}

func (r *SemanticBudgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.SemanticBudget{}).
		Complete(r)
}
