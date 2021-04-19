module github.com/fluxcd/helm-controller/api

go 1.15

require (
	github.com/fluxcd/pkg/apis/kustomize v0.0.1
	github.com/fluxcd/pkg/apis/meta v0.8.0
	github.com/fluxcd/pkg/runtime v0.10.2
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)
