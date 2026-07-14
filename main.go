package main

import (
	"flag"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	securityv1alpha1 "github.com/your-org/ai-workload-security/api/v1alpha1"
	"github.com/your-org/ai-workload-security/controllers"
	webhookv1alpha1 "github.com/your-org/ai-workload-security/webhook/v1alpha1"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(securityv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	var enableWebhook bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "metrics endpoint address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "health probe address")
	flag.BoolVar(&enableWebhook, "enable-webhook", true, "enable the admission webhook server")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions(metricsAddr),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         true,
		LeaderElectionID:       "ai-workload-controller.security.internal",
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.AIPolicyReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create controller", "controller", "AIPolicy")
		os.Exit(1)
	}

	if err = (&controllers.SemanticBudgetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create controller", "controller", "SemanticBudget")
		os.Exit(1)
	}

	aggregator := &controllers.RuntimeAggregator{Client: mgr.GetClient()}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/verdicts", aggregator.HandleVerdict)
	go func() {
		ctrl.Log.Info("starting runtime aggregator endpoint", "addr", ":9443")
		if err := http.ListenAndServe(":9443", mux); err != nil {
			ctrl.Log.Error(err, "runtime aggregator endpoint stopped")
		}
	}()

	if enableWebhook {
		hookServer := mgr.GetWebhookServer()
		hookServer.Register("/validate-agent-manifest", &webhook.Admission{
			Handler: &webhookv1alpha1.AgentDeploymentValidator{Client: mgr.GetClient()},
		})
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctrl.Log.Info("starting AI Workload Controller")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "problem running manager")
		os.Exit(1)
	}
}
