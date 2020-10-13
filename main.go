/*
Copyright 2020 The Flux CD contributors.

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
	"flag"
	"os"
	"time"

	"github.com/go-logr/logr"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	crtlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/metrics"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/helm-controller/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = sourcev1.AddToScheme(scheme)
	_ = v2.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		eventsAddr           string
		enableLeaderElection bool
		concurrent           int
		requeueDependency    time.Duration
		logLevel             string
		logJSON              bool
		watchAllNamespaces   bool
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&eventsAddr, "events-addr", "", "The address of the events receiver.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&concurrent, "concurrent", 4, "The number of concurrent HelmRelease reconciles.")
	flag.DurationVar(&requeueDependency, "requeue-dependency", 30*time.Second, "The interval at which failing dependencies are reevaluated.")
	flag.StringVar(&logLevel, "log-level", "info", "Set logging level. Can be debug, info or error.")
	flag.BoolVar(&logJSON, "log-json", false, "Set logging to JSON format.")
	flag.BoolVar(&watchAllNamespaces, "watch-all-namespaces", true,
		"Watch for custom resources in all namespaces, if set to false it will only watch the runtime namespace.")
	flag.Parse()

	ctrl.SetLogger(newLogger(logLevel, logJSON))

	var eventRecorder *events.Recorder
	if eventsAddr != "" {
		if er, err := events.NewRecorder(eventsAddr, "helm-controller"); err != nil {
			setupLog.Error(err, "unable to create event recorder")
			os.Exit(1)
		} else {
			eventRecorder = er
		}
	}

	metricsRecorder := metrics.NewRecorder()
	crtlmetrics.Registry.MustRegister(metricsRecorder.Collectors()...)

	watchNamespace := ""
	if !watchAllNamespaces {
		watchNamespace = os.Getenv("RUNTIME_NAMESPACE")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "5b6ca942.fluxcd.io",
		Namespace:          watchNamespace,
		Logger:             ctrl.Log,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.HelmChartWatcher{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("HelmChart"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HelmChart")
		os.Exit(1)
	}
	if err = (&controllers.HelmReleaseReconciler{
		Client:                mgr.GetClient(),
		Config:                mgr.GetConfig(),
		Log:                   ctrl.Log.WithName("controllers").WithName(v2.HelmReleaseKind),
		Scheme:                mgr.GetScheme(),
		EventRecorder:         mgr.GetEventRecorderFor("helm-controller"),
		ExternalEventRecorder: eventRecorder,
		MetricsRecorder:       metricsRecorder,
	}).SetupWithManager(mgr, controllers.HelmReleaseReconcilerOptions{
		MaxConcurrentReconciles:   concurrent,
		DependencyRequeueInterval: requeueDependency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", v2.HelmReleaseKind)
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// newLogger returns a logger configured for dev or production use.
// For production the log format is JSON, the timestamps format is ISO8601
// and stack traces are logged when the level is set to debug.
func newLogger(level string, production bool) logr.Logger {
	if !production {
		return zap.New(zap.UseDevMode(true))
	}

	encCfg := uzap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zap.Encoder(zapcore.NewJSONEncoder(encCfg))

	logLevel := zap.Level(zapcore.InfoLevel)
	stacktraceLevel := zap.StacktraceLevel(zapcore.PanicLevel)

	switch level {
	case "debug":
		logLevel = zap.Level(zapcore.DebugLevel)
		stacktraceLevel = zap.StacktraceLevel(zapcore.ErrorLevel)
	case "error":
		logLevel = zap.Level(zapcore.ErrorLevel)
	}

	return zap.New(encoder, logLevel, stacktraceLevel)
}
