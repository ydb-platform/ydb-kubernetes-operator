package main

import (
	"flag"
	"os"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/databasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/monitoring"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotedatabasenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/remotestoragenodeset"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storagenodeset"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ydbv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var disableWebhooks bool
	var enableServiceMonitors bool
	var probeAddr string
	var remoteKubeconfig string
	var remoteCluster string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&disableWebhooks, "disable-webhooks", false, "Disable webhooks registration on start.")
	flag.BoolVar(&enableServiceMonitors, "with-service-monitors", false, "Enables service monitoring")
	flag.StringVar(&remoteKubeconfig, "remote-kubeconfig", "/remote-kubeconfig", "Path to kubeconfig for remote k8s cluster. Only required if using Remote objects")
	flag.StringVar(&remoteCluster, "remote-cluster", "", "The name of remote cluster to sync k8s resources. Only required if using Remote objects")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if enableServiceMonitors {
		utilruntime.Must(monitoringv1.AddToScheme(scheme))
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "a14e577a.ydb.tech",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&database.Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Database")
		os.Exit(1)
	}
	if err = (&storage.Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("ydb-operator"),

		WithServiceMonitors: enableServiceMonitors,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Storage")
		os.Exit(1)
	}
	if err = (&databasenodeset.Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseNodeSet")
		os.Exit(1)
	}
	if err = (&storagenodeset.Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Config:   mgr.GetConfig(),
		Recorder: mgr.GetEventRecorderFor("ydb-operator"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StorageNodeSet")
		os.Exit(1)
	}

	if enableServiceMonitors {
		if err = (&monitoring.DatabaseMonitoringReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DatabaseMonitoring")
			os.Exit(1)
		}
		if err = (&monitoring.StorageMonitoringReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "StorageMonitoring")
			os.Exit(1)
		}
	}

	if !disableWebhooks {
		if err = (&ydbv1alpha1.Storage{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Storage")
			os.Exit(1)
		}
		if err = (&ydbv1alpha1.Database{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Database")
			os.Exit(1)
		}

		if err = ydbv1alpha1.RegisterMonitoringValidatingWebhook(mgr, enableServiceMonitors); err != nil {
			setupLog.Error(err, "unable to create webhooks", "webhooks",
				[]string{"DatabaseMonitoring", "StorageMonitoring"})
			os.Exit(1)
		}
	}

	if remoteKubeconfig != "" && remoteCluster != "" {
		remoteConfig, err := clientcmd.BuildConfigFromFlags("", remoteKubeconfig)
		if err != nil {
			setupLog.Error(err, "unable to read remote kubeconfig")
			os.Exit(1)
		}

		storageSelector, err := remotestoragenodeset.BuildRemoteSelector(remoteCluster)
		if err != nil {
			setupLog.Error(err, "unable to create label selector", "selector", "RemoteStorageNodeSet")
			os.Exit(1)
		}

		databaseSelector, err := remotedatabasenodeset.BuildRemoteSelector(remoteCluster)
		if err != nil {
			setupLog.Error(err, "unable to create label selector", "selector", "RemoteDatabaseNodeSet")
			os.Exit(1)
		}

		remoteCluster, err := cluster.New(remoteConfig, func(o *cluster.Options) {
			o.Scheme = scheme
			o.NewCache = cache.BuilderWithOptions(cache.Options{
				SelectorsByObject: cache.SelectorsByObject{
					&ydbv1alpha1.RemoteStorageNodeSet{}:  {Label: storageSelector},
					&ydbv1alpha1.RemoteDatabaseNodeSet{}: {Label: databaseSelector},
				},
			})
		})
		if err != nil {
			setupLog.Error(err, "unable to create remote client")
			os.Exit(1)
		}

		if err = mgr.Add(remoteCluster); err != nil {
			setupLog.Error(err, "unable to add remote client to controller manager")
			os.Exit(1)
		}

		if err = (&remotestoragenodeset.Reconciler{
			Client:       mgr.GetClient(),
			RemoteClient: remoteCluster.GetClient(),
			Scheme:       mgr.GetScheme(),
		}).SetupWithManager(mgr, &remoteCluster); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "RemoteStorageNodeSet")
			os.Exit(1)
		}

		if err = (&remotedatabasenodeset.Reconciler{
			Client:       mgr.GetClient(),
			RemoteClient: remoteCluster.GetClient(),
			Scheme:       mgr.GetScheme(),
		}).SetupWithManager(mgr, &remoteCluster); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "RemoteDatabaseNodeSet")
			os.Exit(1)
		}
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
