module github.com/fluxcd/helm-controller/api

go 1.15

require (
	github.com/fluxcd/pkg/apis/meta v0.4.0
	github.com/fluxcd/pkg/runtime v0.3.1
	k8s.io/api v0.19.3
	k8s.io/apiextensions-apiserver v0.19.3
	k8s.io/apimachinery v0.19.3
	sigs.k8s.io/controller-runtime v0.6.3
)
