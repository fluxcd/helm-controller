module github.com/fluxcd/helm-controller

go 1.16

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/fluxcd/helm-controller/api v0.13.0
	github.com/fluxcd/pkg/apis/kustomize v0.1.0
	github.com/fluxcd/pkg/apis/meta v0.10.0
	github.com/fluxcd/pkg/runtime v0.12.0
	github.com/fluxcd/source-controller/api v0.18.0
	github.com/go-logr/logr v0.4.0
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/spf13/pflag v1.0.5
	helm.sh/helm/v3 v3.7.1
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/cli-runtime v0.22.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/controller-runtime v0.9.7
	sigs.k8s.io/kustomize/api v0.10.0
	sigs.k8s.io/yaml v1.2.0
)

// Freeze Helm due to OOM issues https://github.com/fluxcd/helm-controller/issues/345
replace helm.sh/helm/v3 => helm.sh/helm/v3 v3.6.3

// Required by https://github.com/helm/helm/blob/v3.6.3/go.mod
replace github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d

// Fix CVE-2021-30465
// Fix CVE-2021-41190
replace github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.2

// Fix CVE-2021-32760
// Fix CVE-2021-41103
// Fix CVE-2021-41190
replace github.com/containerd/containerd => github.com/containerd/containerd v1.4.12
