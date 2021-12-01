module github.com/fluxcd/helm-controller/api

go 1.16

require (
	github.com/fluxcd/pkg/apis/kustomize v0.1.0
	github.com/fluxcd/pkg/apis/meta v0.11.0-rc.1
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	sigs.k8s.io/controller-runtime v0.9.5
)
