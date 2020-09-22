module github.com/fluxcd/helm-controller

go 1.14

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/fluxcd/helm-controller/api v0.0.9
	github.com/fluxcd/pkg/lockedfile v0.0.5
	github.com/fluxcd/pkg/recorder v0.0.5
	github.com/fluxcd/pkg/runtime v0.0.3
	github.com/fluxcd/source-controller/api v0.0.15
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	go.uber.org/zap v1.13.0
	helm.sh/helm/v3 v3.3.3
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/cli-runtime v0.18.8
	k8s.io/client-go v0.18.8
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.6.2
)
