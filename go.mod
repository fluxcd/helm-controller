module github.com/fluxcd/helm-controller

go 1.16

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/fluxcd/helm-controller/api v0.14.1
	github.com/fluxcd/pkg/apis/kustomize v0.3.0
	github.com/fluxcd/pkg/apis/meta v0.10.1
	github.com/fluxcd/pkg/runtime v0.12.2
	github.com/fluxcd/source-controller/api v0.19.2
	github.com/garyburd/redigo v1.6.3 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/spf13/pflag v1.0.5
	helm.sh/helm/v3 v3.7.2
	k8s.io/api v0.22.4
	k8s.io/apiextensions-apiserver v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/cli-runtime v0.22.4
	k8s.io/client-go v0.22.4
	sigs.k8s.io/controller-runtime v0.10.2
	sigs.k8s.io/kustomize/api v0.10.1
	sigs.k8s.io/yaml v1.2.0
)

// Pin kustomize to v4.4.1
replace (
	sigs.k8s.io/kustomize/api => sigs.k8s.io/kustomize/api v0.10.1
	sigs.k8s.io/kustomize/kyaml => sigs.k8s.io/kustomize/kyaml v0.13.0
)

// Fix CVE-2021-41092
// Due to https://github.com/oras-project/oras-go/blob/v0.4.0/go.mod#L14
// pulled in by Helm.
replace github.com/docker/cli => github.com/docker/cli v20.10.9+incompatible

// Fix CVE-2021-30465
// Fix CVE-2021-43784
// Fix GO-2021-0085
// Fix GO-2021-0087
replace github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.3

// Fix CVE-2021-32760
// Fix CVE-2021-41103
// Fix CVE-2021-41190
// Due to https://github.com/oras-project/oras-go/blob/v0.4.0/go.mod#L13,
// pulled in by Helm.
replace github.com/containerd/containerd => github.com/containerd/containerd v1.5.8

// Fix CVE-2021-41190
// Due to https://github.com/oras-project/oras-go/blob/v0.4.0/go.mod#L21,
// pulled in by Helm.
replace github.com/opencontainers/image-spec => github.com/opencontainers/image-spec v1.0.2
