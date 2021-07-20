module github.com/fluxcd/helm-controller

go 1.16

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/fluxcd/helm-controller/api v0.11.1
	github.com/fluxcd/pkg/apis/kustomize v0.1.0
	github.com/fluxcd/pkg/apis/meta v0.11.0-rc.1
	github.com/fluxcd/pkg/runtime v0.13.0-rc.1
	github.com/fluxcd/source-controller/api v0.15.4-0.20210715110516-0dc35980684a
	github.com/go-logr/logr v0.4.0
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/spf13/pflag v1.0.5
	helm.sh/helm/v3 v3.6.1
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/cli-runtime v0.21.1
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.2
	sigs.k8s.io/kustomize/api v0.8.8
	sigs.k8s.io/yaml v1.2.0
)

// required by https://github.com/helm/helm/blob/v3.5.2/go.mod
replace (
	github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
	github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible
)
