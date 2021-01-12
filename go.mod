module github.com/fluxcd/helm-controller

go 1.15

replace github.com/fluxcd/helm-controller/api => ./api

// TODO: Remove once we move to a controller-runtime version which doesn't use this version.
replace github.com/imdario/mergo v0.3.9 => github.com/imdario/mergo v0.3.8

require (
	github.com/fluxcd/helm-controller/api v0.4.4
	github.com/fluxcd/pkg/apis/meta v0.5.0
	github.com/fluxcd/pkg/runtime v0.6.0
	github.com/fluxcd/source-controller/api v0.6.0
	github.com/go-logr/logr v0.3.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/spf13/pflag v1.0.5
	helm.sh/helm/v3 v3.4.2
	k8s.io/api v0.19.4
	k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/cli-runtime v0.19.4
	k8s.io/client-go v0.19.4
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/yaml v1.2.0
)
