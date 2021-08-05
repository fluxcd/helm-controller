module github.com/fluxcd/helm-controller

go 1.16

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/fluxcd/helm-controller/api v0.11.2
	github.com/fluxcd/pkg/apis/kustomize v0.1.0
	github.com/fluxcd/pkg/apis/meta v0.10.0
	github.com/fluxcd/pkg/runtime v0.12.0
	github.com/fluxcd/source-controller/api v0.15.4
	github.com/go-logr/logr v0.4.0
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.14.0
	github.com/spf13/pflag v1.0.5
	helm.sh/helm/v3 v3.6.3
	k8s.io/api v0.21.3
	k8s.io/apiextensions-apiserver v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/cli-runtime v0.21.3
	k8s.io/client-go v0.21.3
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/kustomize/api v0.8.8
	sigs.k8s.io/yaml v1.2.0
)

// required by https://github.com/helm/helm/blob/v3.6.3/go.mod
replace github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
