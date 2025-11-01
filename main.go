/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"time"

	flag "github.com/spf13/pflag"
	"github.com/matheuscscp/helm/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/fluxcd/pkg/auth"
	"github.com/fluxcd/pkg/cache"
	"github.com/fluxcd/pkg/runtime/acl"
	"github.com/fluxcd/pkg/runtime/client"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/events"
	feathelper "github.com/fluxcd/pkg/runtime/features"
	"github.com/fluxcd/pkg/runtime/jitter"
	"github.com/fluxcd/pkg/runtime/leaderelection"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/pprof"
	"github.com/fluxcd/pkg/runtime/probes"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	intdigest "github.com/fluxcd/helm-controller/internal/digest"

	// +kubebuilder:scaffold:imports

	intacl "github.com/fluxcd/helm-controller/internal/acl"
	"github.com/fluxcd/helm-controller/internal/controller"
	"github.com/fluxcd/helm-controller/internal/features"
	intkube "github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/oomwatch"
)

const controllerName = "helm-controller"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(sourcev1.AddToScheme(scheme))
	utilruntime.Must(v2.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	const (
		tokenCacheDefaultMaxSize = 100
	)

	var (
		metricsAddr                     string
		eventsAddr                      string
		healthAddr                      string
		concurrent                      int
		requeueDependency               time.Duration
		gracefulShutdownTimeout         time.Duration
		httpRetry                       int
		clientOptions                   client.Options
		kubeConfigOpts                  client.KubeConfigOptions
		featureGates                    feathelper.FeatureGates
		logOptions                      logger.Options
		aclOptions                      acl.Options
		leaderElectionOptions           leaderelection.Options
		rateLimiterOptions              helper.RateLimiterOptions
		watchOptions                    helper.WatchOptions
		intervalJitterOptions           jitter.IntervalOptions
		oomWatchInterval                time.Duration
		oomWatchMemoryThreshold         uint8
		oomWatchMaxMemoryPath           string
		oomWatchCurrentMemoryPath       string
		snapshotDigestAlgo              string
		tokenCacheOptions               cache.TokenFlags
		defaultKubeConfigServiceAccount string
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080",
		"The address the metric endpoint binds to.")
	flag.StringVar(&eventsAddr, "events-addr", "",
		"The address of the events receiver.")
	flag.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	flag.IntVar(&concurrent, "concurrent", 4,
		"The number of concurrent HelmRelease reconciles.")
	flag.DurationVar(&requeueDependency, "requeue-dependency", 30*time.Second,
		"The interval at which failing dependencies are reevaluated.")
	flag.DurationVar(&gracefulShutdownTimeout, "graceful-shutdown-timeout", 600*time.Second,
		"The duration given to the reconciler to finish before forcibly stopping.")
	flag.IntVar(&httpRetry, "http-retry", 9,
		"The maximum number of retries when failing to fetch artifacts over HTTP.")
	flag.StringVar(&intkube.DefaultServiceAccountName, auth.ControllerFlagDefaultServiceAccount, "",
		"Default service account used for impersonation.")
	flag.StringVar(&defaultKubeConfigServiceAccount, auth.ControllerFlagDefaultKubeConfigServiceAccount, "",
		"Default service account used for kubeconfig.")
	flag.Uint8Var(&oomWatchMemoryThreshold, "oom-watch-memory-threshold", 95,
		"The memory threshold in percentage at which the OOM watcher will trigger a graceful shutdown. Requires feature gate 'OOMWatch' to be enabled.")
	flag.DurationVar(&oomWatchInterval, "oom-watch-interval", 500*time.Millisecond,
		"The interval at which the OOM watcher will check for memory usage. Requires feature gate 'OOMWatch' to be enabled.")
	flag.StringVar(&oomWatchMaxMemoryPath, "oom-watch-max-memory-path", "",
		"The path to the cgroup memory limit file. Requires feature gate 'OOMWatch' to be enabled. If not set, the path will be automatically detected.")
	flag.StringVar(&oomWatchCurrentMemoryPath, "oom-watch-current-memory-path", "",
		"The path to the cgroup current memory usage file. Requires feature gate 'OOMWatch' to be enabled. If not set, the path will be automatically detected.")
	flag.StringVar(&snapshotDigestAlgo, "snapshot-digest-algo", intdigest.Canonical.String(),
		"The algorithm to use to calculate the digest of Helm release storage snapshots.")

	clientOptions.BindFlags(flag.CommandLine)
	logOptions.BindFlags(flag.CommandLine)
	aclOptions.BindFlags(flag.CommandLine)
	leaderElectionOptions.BindFlags(flag.CommandLine)
	rateLimiterOptions.BindFlags(flag.CommandLine)
	kubeConfigOpts.BindFlags(flag.CommandLine)
	featureGates.BindFlags(flag.CommandLine)
	watchOptions.BindFlags(flag.CommandLine)
	intervalJitterOptions.BindFlags(flag.CommandLine)
	tokenCacheOptions.BindFlags(flag.CommandLine, tokenCacheDefaultMaxSize)

	flag.Parse()

	logger.SetLogger(logger.NewLogger(logOptions))

	err := featureGates.WithLogger(setupLog).
		SupportedFeatures(features.FeatureGates())
	if err != nil {
		setupLog.Error(err, "unable to load feature gates")
		os.Exit(1)
	}

	switch enabled, err := features.Enabled(auth.FeatureGateObjectLevelWorkloadIdentity); {
	case err != nil:
		setupLog.Error(err, "unable to check feature gate "+auth.FeatureGateObjectLevelWorkloadIdentity)
		os.Exit(1)
	case enabled:
		auth.EnableObjectLevelWorkloadIdentity()
	}

	if defaultKubeConfigServiceAccount != "" {
		auth.SetDefaultKubeConfigServiceAccount(defaultKubeConfigServiceAccount)
	}

	if auth.InconsistentObjectLevelConfiguration() {
		setupLog.Error(auth.ErrInconsistentObjectLevelConfiguration, "invalid configuration")
		os.Exit(1)
	}

	if err := intervalJitterOptions.SetGlobalJitter(nil); err != nil {
		setupLog.Error(err, "unable to set global jitter")
		os.Exit(1)
	}

	watchNamespace := ""
	if !watchOptions.AllNamespaces {
		watchNamespace = os.Getenv("RUNTIME_NAMESPACE")
	}

	watchSelector, err := helper.GetWatchSelector(watchOptions)
	if err != nil {
		setupLog.Error(err, "unable to configure watch label selector for manager")
		os.Exit(1)
	}

	watchConfigsPredicate, err := helper.GetWatchConfigsPredicate(watchOptions)
	if err != nil {
		setupLog.Error(err, "unable to configure watch configs label selector for controller")
		os.Exit(1)
	}

	var disableCacheFor []ctrlclient.Object
	shouldCache, err := features.Enabled(features.CacheSecretsAndConfigMaps)
	if err != nil {
		setupLog.Error(err, "unable to check feature gate CacheSecretsAndConfigMaps")
		os.Exit(1)
	}
	if !shouldCache {
		disableCacheFor = append(disableCacheFor, &corev1.Secret{}, &corev1.ConfigMap{})
	}

	leaderElectionId := fmt.Sprintf("%s-%s", controllerName, "leader-election")
	if watchOptions.LabelSelector != "" {
		leaderElectionId = leaderelection.GenerateID(leaderElectionId, watchOptions.LabelSelector)
	}

	disableChartDigestTracking, err := features.Enabled(features.DisableChartDigestTracking)
	if err != nil {
		setupLog.Error(err, "unable to check feature gate "+features.DisableChartDigestTracking)
		os.Exit(1)
	}

	additiveCELDependencyCheck, err := features.Enabled(features.AdditiveCELDependencyCheck)
	if err != nil {
		setupLog.Error(err, "unable to check feature gate "+features.AdditiveCELDependencyCheck)
		os.Exit(1)
	}

	// Set the managedFields owner for resources reconciled from Helm charts.
	kube.ManagedFieldsManager = controllerName

	// Configure the ACL policy.
	intacl.AllowCrossNamespaceRef = !aclOptions.NoCrossNamespaceRefs

	// Configure the digest algorithm.
	if snapshotDigestAlgo != intdigest.Canonical.String() {
		algo, err := intdigest.AlgorithmForName(snapshotDigestAlgo)
		if err != nil {
			setupLog.Error(err, "unable to configure canonical digest algorithm")
			os.Exit(1)
		}
		intdigest.Canonical = algo
	}

	restConfig := client.GetConfigOrDie(clientOptions)

	mgrConfig := ctrl.Options{
		Scheme:                        scheme,
		HealthProbeBindAddress:        healthAddr,
		LeaderElection:                leaderElectionOptions.Enable,
		LeaderElectionReleaseOnCancel: leaderElectionOptions.ReleaseOnCancel,
		LeaseDuration:                 &leaderElectionOptions.LeaseDuration,
		RenewDeadline:                 &leaderElectionOptions.RenewDeadline,
		RetryPeriod:                   &leaderElectionOptions.RetryPeriod,
		GracefulShutdownTimeout:       &gracefulShutdownTimeout,
		LeaderElectionID:              leaderElectionId,
		Logger:                        ctrl.Log,
		Client: ctrlclient.Options{
			Cache: &ctrlclient.CacheOptions{
				DisableFor: disableCacheFor,
			},
		},
		Cache: ctrlcache.Options{
			ByObject: map[ctrlclient.Object]ctrlcache.ByObject{
				&v2.HelmRelease{}: {Label: watchSelector},
			},
		},
		Controller: ctrlcfg.Controller{
			RecoverPanic:            ptr.To(true),
			MaxConcurrentReconciles: concurrent,
		},
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			ExtraHandlers: pprof.GetHandlers(),
		},
	}

	if watchNamespace != "" {
		mgrConfig.Cache.DefaultNamespaces = map[string]ctrlcache.Config{
			watchNamespace: ctrlcache.Config{},
		}
	}

	mgr, err := ctrl.NewManager(restConfig, mgrConfig)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	probes.SetupChecks(mgr, setupLog)

	metricsH := helper.NewMetrics(mgr, metrics.MustMakeRecorder(), v2.HelmReleaseFinalizer)
	var eventRecorder *events.Recorder
	if eventRecorder, err = events.NewRecorder(mgr, ctrl.Log, eventsAddr, controllerName); err != nil {
		setupLog.Error(err, "unable to create event recorder")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	if ok, _ := features.Enabled(features.OOMWatch); ok {
		setupLog.Info("setting up OOM watcher")
		ow, err := oomwatch.New(
			oomWatchMaxMemoryPath,
			oomWatchCurrentMemoryPath,
			oomWatchMemoryThreshold,
			oomWatchInterval,
			ctrl.Log.WithName("OOMwatch"),
		)
		if err != nil {
			setupLog.Error(err, "unable to setup OOM watcher")
			os.Exit(1)
		}
		ctx = ow.Watch(ctx)
	}

	var tokenCache *cache.TokenCache
	if tokenCacheOptions.MaxSize > 0 {
		var err error
		tokenCache, err = cache.NewTokenCache(tokenCacheOptions.MaxSize,
			cache.WithMaxDuration(tokenCacheOptions.MaxDuration),
			cache.WithMetricsRegisterer(ctrlmetrics.Registry),
			cache.WithMetricsPrefix("gotk_token_"))
		if err != nil {
			setupLog.Error(err, "unable to create token cache")
			os.Exit(1)
		}
	}

	allowExternalArtifact, err := features.Enabled(features.ExternalArtifact)
	if err != nil {
		setupLog.Error(err, "unable to check feature gate "+features.ExternalArtifact)
		os.Exit(1)
	}

	if err = (&controller.HelmReleaseReconciler{
		Client:                     mgr.GetClient(),
		APIReader:                  mgr.GetAPIReader(),
		EventRecorder:              eventRecorder,
		Metrics:                    metricsH,
		GetClusterConfig:           ctrl.GetConfig,
		ClientOpts:                 clientOptions,
		KubeConfigOpts:             kubeConfigOpts,
		FieldManager:               controllerName,
		DisableChartDigestTracking: disableChartDigestTracking,
		AdditiveCELDependencyCheck: additiveCELDependencyCheck,
		TokenCache:                 tokenCache,
		DependencyRequeueInterval:  requeueDependency,
		ArtifactFetchRetries:       httpRetry,
		AllowExternalArtifact:      allowExternalArtifact,
	}).SetupWithManager(ctx, mgr, controller.HelmReleaseReconcilerOptions{
		RateLimiter:            helper.GetRateLimiter(rateLimiterOptions),
		WatchExternalArtifacts: allowExternalArtifact,
		WatchConfigsPredicate:  watchConfigsPredicate,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", v2.HelmReleaseKind)
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
