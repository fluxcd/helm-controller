module github.com/fluxcd/helm-controller

go 1.15

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/fluxcd/helm-controller/api v0.2.2
	github.com/fluxcd/pkg/apis/meta v0.4.0
	github.com/fluxcd/pkg/runtime v0.3.0
	github.com/fluxcd/source-controller/api v0.3.0
	github.com/go-logr/logr v0.2.1
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	go.uber.org/zap v1.13.0
	helm.sh/helm/v3 v3.4.0
	k8s.io/api v0.19.3
	k8s.io/apiextensions-apiserver v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/cli-runtime v0.19.3
	k8s.io/client-go v0.19.3
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.6.3
	sigs.k8s.io/yaml v1.2.0
)
