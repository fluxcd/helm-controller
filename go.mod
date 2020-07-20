module github.com/fluxcd/helm-controller

go 1.14

require (
	github.com/fluxcd/pkg v0.0.3
	github.com/fluxcd/source-controller v0.0.5
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	go.uber.org/zap v1.10.0
	helm.sh/helm/v3 v3.2.4
	k8s.io/api v0.18.4
	k8s.io/apiextensions-apiserver v0.18.4
	k8s.io/apimachinery v0.18.4
	k8s.io/cli-runtime v0.18.4
	k8s.io/client-go v0.18.4
	sigs.k8s.io/controller-runtime v0.6.1
)
