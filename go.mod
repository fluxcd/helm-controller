module github.com/fluxcd/helm-controller

go 1.16

replace github.com/fluxcd/helm-controller/api => ./api

require (
	github.com/bshuster-repo/logrus-logstash-hook v1.0.2 // indirect
	github.com/bugsnag/bugsnag-go v2.1.2+incompatible // indirect
	github.com/bugsnag/panicwrap v1.3.4 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/fluxcd/helm-controller/api v0.13.0
	github.com/fluxcd/pkg/apis/kustomize v0.1.0
	github.com/fluxcd/pkg/apis/meta v0.10.0
	github.com/fluxcd/pkg/runtime v0.12.0
	github.com/fluxcd/source-controller/api v0.18.0
	github.com/garyburd/redigo v1.6.3 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/gofrs/uuid v4.1.0+incompatible // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/spf13/pflag v1.0.5
	github.com/yvasiyarov/go-metrics v0.0.0-20150112132944-c25f46c4b940 // indirect
	github.com/yvasiyarov/gorelic v0.0.7 // indirect
	github.com/yvasiyarov/newrelic_platform_go v0.0.0-20160601141957-9c099fbc30e9 // indirect
	helm.sh/helm/v3 v3.7.1
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/cli-runtime v0.22.1
	k8s.io/client-go v0.22.1
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.9.7
	sigs.k8s.io/kustomize/api v0.10.0
	sigs.k8s.io/yaml v1.2.0
)

// Freeze Helm due to OOM issues https://github.com/fluxcd/helm-controller/issues/345
replace helm.sh/helm/v3 => helm.sh/helm/v3 v3.6.3

// Required by https://github.com/helm/helm/blob/v3.6.3/go.mod,
// but overwritten with a newer version due to CVE-2017-11468.
replace github.com/docker/distribution => github.com/docker/distribution v2.7.0-rc.0+incompatible

// Fix CVE-2021-41092
replace github.com/docker/cli => github.com/docker/cli v20.10.9+incompatible

// Fix CVE-2021-30465
replace github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.2

// Fix CVE-2021-32760
// Fix CVE-2021-41103
// Fix CVE-2021-41190
replace github.com/containerd/containerd => github.com/containerd/containerd v1.4.12

// Fix CVE-2021-41190
replace github.com/opencontainers/image-spec => github.com/opencontainers/image-spec v1.0.2
