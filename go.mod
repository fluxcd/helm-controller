module github.com/fluxcd/helm-controller

go 1.13

require (
	github.com/fluxcd/source-controller v0.0.1
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	helm.sh/helm/v3 v3.2.4
	k8s.io/api v0.18.4
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.4
	k8s.io/cli-runtime v0.18.0
	k8s.io/client-go v0.18.2
	sigs.k8s.io/controller-runtime v0.6.0
)
