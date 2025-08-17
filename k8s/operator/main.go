package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	kvstoreiov1 "github.com/kvstore/operator/api/v1"
	"github.com/kvstore/operator/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kvstoreiov1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// OperatorConfig holds the configuration for the operator
type OperatorConfig struct {
	MetricsAddr          string
	EnableLeaderElection bool
	LeaderElectionID     string
	ProbeAddr           string
	WebhookPort         int
	CertDir             string
	Namespace           string
	WatchNamespace      string
	MaxConcurrentReconciles int
	SyncPeriod          time.Duration
	LogLevel            string
	Development         bool
}

func main() {
	var config OperatorConfig
	flag.StringVar(&config.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&config.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&config.EnableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&config.LeaderElectionID, "leader-election-id", "kvstore-operator-lock",
		"Name of the configmap used for leader election.")
	flag.IntVar(&config.WebhookPort, "webhook-port", 9443, "Port for the webhook server.")
	flag.StringVar(&config.CertDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", 
		"Directory containing TLS certificates for webhooks.")
	flag.StringVar(&config.Namespace, "namespace", "", "Namespace to watch for resources (empty for all namespaces).")
	flag.StringVar(&config.WatchNamespace, "watch-namespace", "", "Namespace to watch (defaults to all namespaces).")
	flag.IntVar(&config.MaxConcurrentReconciles, "max-concurrent-reconciles", 1, 
		"Maximum number of concurrent reconciles per controller.")
	flag.DurationVar(&config.SyncPeriod, "sync-period", 10*time.Minute, "Controller sync period.")
	flag.StringVar(&config.LogLevel, "log-level", "info", "Log level (debug, info, warn, error).")
	flag.BoolVar(&config.Development, "development", false, "Enable development mode with more verbose logging.")
	
	opts := zap.Options{
		Development: config.Development,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Setup manager options
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     config.MetricsAddr,
		Port:                   config.WebhookPort,
		HealthProbeBindAddress: config.ProbeAddr,
		LeaderElection:         config.EnableLeaderElection,
		LeaderElectionID:       config.LeaderElectionID,
		Namespace:              config.Namespace,
		SyncPeriod:             &config.SyncPeriod,
		CertDir:               config.CertDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup controllers
	if err = setupControllers(mgr, config); err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}

	// Setup webhooks
	if err = setupWebhooks(mgr); err != nil {
		setupLog.Error(err, "unable to setup webhooks")
		os.Exit(1)
	}

	// Add health and readiness checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Register custom metrics
	registerMetrics()

	setupLog.Info("starting manager", 
		"version", getVersion(),
		"namespace", config.Namespace,
		"leaderElection", config.EnableLeaderElection,
	)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupControllers sets up all the controllers with the Manager
func setupControllers(mgr manager.Manager, config OperatorConfig) error {
	// Setup KVStore controller
	if err := (&controllers.KVStoreReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("kvstore-controller"),
		Log:      ctrl.Log.WithName("controllers").WithName("KVStore"),
		Config:   convertToReconcilerConfig(config),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KVStore controller: %w", err)
	}

	// Setup KVStoreBackup controller
	if err := (&controllers.KVStoreBackupReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("kvstore-backup-controller"),
		Log:      ctrl.Log.WithName("controllers").WithName("KVStoreBackup"),
		Config:   convertToReconcilerConfig(config),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KVStoreBackup controller: %w", err)
	}

	// Setup KVStoreRestore controller
	if err := (&controllers.KVStoreRestoreReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("kvstore-restore-controller"),
		Log:      ctrl.Log.WithName("controllers").WithName("KVStoreRestore"),
		Config:   convertToReconcilerConfig(config),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KVStoreRestore controller: %w", err)
	}

	return nil
}

// setupWebhooks sets up the webhooks with the Manager
func setupWebhooks(mgr manager.Manager) error {
	// Setup webhook server
	webhookServer := &webhook.Server{
		Port:    9443,
		CertDir: "/tmp/k8s-webhook-server/serving-certs",
	}
	mgr.Add(webhookServer)

	// Register validation and mutation webhooks
	if err := (&kvstoreiov1.KVStore{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KVStore webhook: %w", err)
	}

	if err := (&kvstoreiov1.KVStoreBackup{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KVStoreBackup webhook: %w", err)
	}

	if err := (&kvstoreiov1.KVStoreRestore{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KVStoreRestore webhook: %w", err)
	}

	return nil
}

// convertToReconcilerConfig converts OperatorConfig to ReconcilerConfig
func convertToReconcilerConfig(config OperatorConfig) controllers.ReconcilerConfig {
	return controllers.ReconcilerConfig{
		MaxConcurrentReconciles: config.MaxConcurrentReconciles,
		Namespace:              config.Namespace,
		WatchNamespace:         config.WatchNamespace,
		SyncPeriod:            config.SyncPeriod,
		Development:           config.Development,
	}
}

// registerMetrics registers custom metrics with the metrics registry
func registerMetrics() {
	// Register operator-specific metrics
	operatorMetrics := &controllers.OperatorMetrics{}
	operatorMetrics.Register(metrics.Registry)
}

// getVersion returns the operator version
func getVersion() string {
	// This would typically be set at build time
	return "v1.0.0"
}