module github.com/fluxcd/helm-controller/api

go 1.15

require (
	github.com/fluxcd/pkg/apis/meta v0.0.2
	github.com/fluxcd/pkg/runtime v0.0.6
	k8s.io/api v0.18.9
	k8s.io/apiextensions-apiserver v0.18.9
	k8s.io/apimachinery v0.18.9
	sigs.k8s.io/controller-runtime v0.6.3
)
