# Helm Releases

<!-- menuweight:20 -->

The `HelmRelease` API defines a resource for automated controller driven Helm releases.

## Specification

A **HelmRelease** object defines a resource for controller driven reconciliation
of Helm releases via Helm actions such as install, upgrade, test, uninstall, and rollback.
This includes release placement (namespace/name), release content (chart/values overrides),
action trigger configuration, individual action configuration, and statusing.

```go
// HelmReleaseSpec defines the desired state of a Helm Release.
type HelmReleaseSpec struct {
	// Chart defines the template of the v1beta1.HelmChart that should be created
	// for this HelmRelease.
	// +required
	Chart HelmChartTemplate `json:"chart"`

	// Interval at which to reconcile the Helm release.
	// This interval is approximate and may be subject to jitter to ensure
	// efficient use of resources.
	// +required
	Interval metav1.Duration `json:"interval"`

	// Suspend tells the controller to suspend reconciliation for this HelmRelease,
	// it does not apply to already started reconciliations. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// ReleaseName used for the Helm release. Defaults to a composition of
	// '[TargetNamespace-]Name'.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=53
	// +kubebuilder:validation:Optional
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// TargetNamespace to target when performing operations for the HelmRelease.
	// Defaults to the namespace of the HelmRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// StorageNamespace used for the Helm storage.
	// Defaults to the namespace of the HelmRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	StorageNamespace string `json:"storageNamespace,omitempty"`

	// DependsOn may contain a dependency.CrossNamespaceDependencyReference slice with
	// references to HelmRelease resources that must be ready before this HelmRelease
	// can be reconciled.
	// +optional
	DependsOn []dependency.CrossNamespaceDependencyReference `json:"dependsOn,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm action. Defaults to '5m0s'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// MaxHistory is the number of revisions saved by Helm for this HelmRelease.
	// Use '0' for an unlimited number of revisions; defaults to '10'.
	// +optional
	MaxHistory *int `json:"maxHistory,omitempty"`

	// PersistentClient tells the controller to use a persistent Kubernetes
	// client for this release. When enabled, the client will be reused for the
	// duration of the reconciliation, instead of being created and destroyed
	// for each (step of a) Helm action.
	//
	// This can improve performance, but may cause issues with some Helm charts
	// that for example do create Custom Resource Definitions during installation
	// outside Helm's CRD lifecycle hooks, which are then not observed to be
	// available by e.g. post-install hooks.
	//
	// If not set, it defaults to true.
	//
	// +optional
	PersistentClient *bool `json:"persistentClient,omitempty"`

	// Install holds the configuration for Helm install actions for this HelmRelease.
	// +optional
	Install *Install `json:"install,omitempty"`

	// Upgrade holds the configuration for Helm upgrade actions for this HelmRelease.
	// +optional
	Upgrade *Upgrade `json:"upgrade,omitempty"`

	// Test holds the configuration for Helm test actions for this HelmRelease.
	// +optional
	Test *Test `json:"test,omitempty"`

	// Rollback holds the configuration for Helm rollback actions for this HelmRelease.
	// +optional
	Rollback *Rollback `json:"rollback,omitempty"`

	// Uninstall holds the configuration for Helm uninstall actions for this HelmRelease.
	// +optional
	Uninstall *Uninstall `json:"uninstall,omitempty"`

	// ValuesFrom holds references to resources containing Helm values for this HelmRelease,
	// and information about how they should be merged.
	ValuesFrom []ValuesReference `json:"valuesFrom,omitempty"`

	// Values holds the values for this Helm release.
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// KubeConfig for reconciling the HelmRelease on a remote cluster.
	// When used in combination with HelmReleaseSpec.ServiceAccountName,
	// forces the controller to act on behalf of that Service Account at the
	// target cluster.
	// If the --default-service-account flag is set, its value will be used as
	// a controller level fallback for when HelmReleaseSpec.ServiceAccountName
	// is empty.
	// +optional
	KubeConfig *KubeConfig `json:"kubeConfig,omitempty"`

	// The name of the Kubernetes service account to impersonate
	// when reconciling this HelmRelease.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// PostRenderers holds an array of Helm PostRenderers, which will be applied in order
	// of their definition.
	// +optional
	PostRenderers []PostRenderer `json:"postRenderers,omitempty"`
}

// KubeConfig references a Kubernetes secret that contains a kubeconfig file.
type KubeConfig struct {
	// SecretRef holds the name to a secret that contains a key with
	// the kubeconfig file as the value. If no key is specified the key will
	// default to 'value'. The secret must be in the same namespace as
	// the HelmRelease.
	// It is recommended that the kubeconfig is self-contained, and the secret
	// is regularly updated if credentials such as a cloud-access-token expire.
	// Cloud specific `cmd-path` auth helpers will not function without adding
	// binaries and credentials to the Pod that is responsible for reconciling
	// the HelmRelease.
	// +required
	SecretRef meta.SecretKeyReference `json:"secretRef,omitempty"`
}

// HelmChartTemplate defines the template from which the controller will
// generate a v1beta1.HelmChart object in the same namespace as the referenced
// v1beta1.Source.
type HelmChartTemplate struct {
	// ObjectMeta holds the template for metadata like labels and annotations.
	// +optional
	ObjectMeta *HelmChartTemplateObjectMeta `json:"metadata,omitempty"`

	// Spec holds the template for the v1beta1.HelmChartSpec for this HelmRelease.
	// +required
	Spec HelmChartTemplateSpec `json:"spec"`
}

// HelmChartTemplateObjectMeta defines the template for the ObjectMeta of a
// v1beta2.HelmChart.
type HelmChartTemplateObjectMeta struct {
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// HelmChartTemplateSpec defines the template from which the controller will
// generate a v1beta1.HelmChartSpec object.
type HelmChartTemplateSpec struct {
	// The name or path the Helm chart is available at in the SourceRef.
	// +required
	Chart string `json:"chart"`

	// Version semver expression, ignored for charts from v1beta1.GitRepository and
	// v1beta1.Bucket sources. Defaults to latest when omitted.
	// +kubebuilder:default:=*
	// +optional
	Version string `json:"version,omitempty"`

	// The name and namespace of the v1beta1.Source the chart is available at.
	// +required
	SourceRef CrossNamespaceObjectReference `json:"sourceRef"`

	// Interval at which to check the v1beta1.Source for updates. Defaults to
	// 'HelmReleaseSpec.Interval'.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// Alternative list of values files to use as the chart values (values.yaml
	// is not included by default), expected to be a relative path in the SourceRef.
	// Values files are merged in the order of this list with the last file overriding
	// the first. Ignored when omitted.
	// +optional
	ValuesFiles []string `json:"valuesFiles,omitempty"`

	// Alternative values file to use as the default chart values, expected to
	// be a relative path in the SourceRef. Deprecated in favor of ValuesFiles,
	// for backwards compatibility the file defined here is merged before the
	// ValuesFiles items. Ignored when omitted.
	// +optional
	// +deprecated
	ValuesFile string `json:"valuesFile,omitempty"`
}

// Install holds the configuration for Helm install actions performed for this
// HelmRelease.
type Install struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm install action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Remediation holds the remediation configuration for when the Helm install
	// action for the HelmRelease fails. The default is to not perform any action.
	// +optional
	Remediation *InstallRemediation `json:"remediation,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a Helm
	// install has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableWaitForJobs disables waiting for jobs to complete after a Helm
	// install has been performed.
	// +optional
	DisableWaitForJobs bool `json:"disableWaitForJobs,omitempty"`

	// DisableHooks prevents hooks from running during the Helm install action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// DisableOpenAPIValidation prevents the Helm install action from validating
	// rendered templates against the Kubernetes OpenAPI Schema.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Replace tells the Helm install action to re-use the 'ReleaseName', but only
	// if that name is a deleted release which remains in the history.
	// +optional
	Replace bool `json:"replace,omitempty"`

	// SkipCRDs tells the Helm install action to not install any CRDs. By default,
	// CRDs are installed if not already present.
	//
	// Deprecated use CRD policy (`crds`) attribute with value `Skip` instead.
	//
	// +deprecated
	// +optional
	SkipCRDs bool `json:"skipCRDs,omitempty"`

	// CRDs upgrade CRDs from the Helm Chart's crds directory according
	// to the CRD upgrade policy provided here. Valid values are `Skip`,
	// `Create` or `CreateReplace`. Default is `Create` and if omitted
	// CRDs are installed but not updated.
	//
	// Skip: do neither install nor replace (update) any CRDs.
	//
	// Create: new CRDs are created, existing CRDs are neither updated nor deleted.
	//
	// CreateReplace: new CRDs are created, existing CRDs are updated (replaced)
	// but not deleted.
	//
	// By default, CRDs are applied (installed) during Helm install action.
	// With this option users can opt-in to CRD replace existing CRDs on Helm
	// install actions, which is not (yet) natively supported by Helm.
	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions.
	//
	// +kubebuilder:validation:Enum=Skip;Create;CreateReplace
	// +optional
	CRDs CRDsPolicy `json:"crds,omitempty"`

	// CreateNamespace tells the Helm install action to create the
	// HelmReleaseSpec.TargetNamespace if it does not exist yet.
	// On uninstall, the namespace will not be garbage collected.
	// +optional
	CreateNamespace bool `json:"createNamespace,omitempty"`
}

// InstallRemediation holds the configuration for Helm install remediation.
type InstallRemediation struct {
	// Retries is the number of retries that should be attempted on failures before
	// bailing. Remediation, using an uninstall, is performed between each attempt.
	// Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	Retries int `json:"retries,omitempty"`

	// IgnoreTestFailures tells the controller to skip remediation when the Helm
	// tests are run after an install action but fail. Defaults to
	// 'Test.IgnoreFailures'.
	// +optional
	IgnoreTestFailures *bool `json:"ignoreTestFailures,omitempty"`

	// RemediateLastFailure tells the controller to remediate the last failure, when
	// no retries remain. Defaults to 'false'.
	// +optional
	RemediateLastFailure *bool `json:"remediateLastFailure,omitempty"`
}

// Upgrade holds the configuration for Helm upgrade actions for this
// HelmRelease.
type Upgrade struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm upgrade action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Remediation holds the remediation configuration for when the Helm upgrade
	// action for the HelmRelease fails. The default is to not perform any action.
	// +optional
	Remediation *UpgradeRemediation `json:"remediation,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a Helm
	// upgrade has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableWaitForJobs disables waiting for jobs to complete after a Helm
	// upgrade has been performed.
	// +optional
	DisableWaitForJobs bool `json:"disableWaitForJobs,omitempty"`

	// DisableHooks prevents hooks from running during the Helm upgrade action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// DisableOpenAPIValidation prevents the Helm upgrade action from validating
	// rendered templates against the Kubernetes OpenAPI Schema.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Force forces resource updates through a replacement strategy.
	// +optional
	Force bool `json:"force,omitempty"`

	// PreserveValues will make Helm reuse the last release's values and merge in
	// overrides from 'Values'. Setting this flag makes the HelmRelease
	// non-declarative.
	// +optional
	PreserveValues bool `json:"preserveValues,omitempty"`

	// CleanupOnFail allows deletion of new resources created during the Helm
	// upgrade action when it fails.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`

	// CRDs upgrade CRDs from the Helm Chart's crds directory according
	// to the CRD upgrade policy provided here. Valid values are `Skip`,
	// `Create` or `CreateReplace`. Default is `Skip` and if omitted
	// CRDs are neither installed nor upgraded.
	//
	// Skip: do neither install nor replace (update) any CRDs.
	//
	// Create: new CRDs are created, existing CRDs are neither updated nor deleted.
	//
	// CreateReplace: new CRDs are created, existing CRDs are updated (replaced)
	// but not deleted.
	//
	// By default, CRDs are not applied during Helm upgrade action. With this
	// option users can opt-in to CRD upgrade, which is not (yet) natively supported by Helm.
	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions.
	//
	// +kubebuilder:validation:Enum=Skip;Create;CreateReplace
	// +optional
	CRDs CRDsPolicy `json:"crds,omitempty"`
}

// UpgradeRemediation holds the configuration for Helm upgrade remediation.
type UpgradeRemediation struct {
	// Retries is the number of retries that should be attempted on failures before
	// bailing. Remediation, using 'Strategy', is performed between each attempt.
	// Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	Retries int `json:"retries,omitempty"`

	// IgnoreTestFailures tells the controller to skip remediation when the Helm
	// tests are run after an upgrade action but fail.
	// Defaults to 'Test.IgnoreFailures'.
	// +optional
	IgnoreTestFailures *bool `json:"ignoreTestFailures,omitempty"`

	// RemediateLastFailure tells the controller to remediate the last failure, when
	// no retries remain. Defaults to 'false' unless 'Retries' is greater than 0.
	// +optional
	RemediateLastFailure *bool `json:"remediateLastFailure,omitempty"`

	// Strategy to use for failure remediation. Defaults to 'rollback'.
	// +kubebuilder:validation:Enum=rollback;uninstall
	// +optional
	Strategy *RemediationStrategy `json:"strategy,omitempty"`
}

// Test holds the configuration for Helm test actions for this HelmRelease.
type Test struct {
	// Enable enables Helm test actions for this HelmRelease after an Helm install
	// or upgrade action has been performed.
	// +optional
	Enable bool `json:"enable,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation during
	// the performance of a Helm test action. Defaults to 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// IgnoreFailures tells the controller to skip remediation when the Helm tests
	// are run but fail. Can be overwritten for tests run after install or upgrade
	// actions in 'Install.IgnoreTestFailures' and 'Upgrade.IgnoreTestFailures'.
	// +optional
	IgnoreFailures bool `json:"ignoreFailures,omitempty"`
}

// Rollback holds the configuration for Helm rollback actions for this
// HelmRelease.
type Rollback struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm rollback action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a Helm
	// rollback has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableWaitForJobs disables waiting for jobs to complete after a Helm
	// rollback has been performed.
	// +optional
	DisableWaitForJobs bool `json:"disableWaitForJobs,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// Recreate performs pod restarts for the resource if applicable.
	// +optional
	Recreate bool `json:"recreate,omitempty"`

	// Force forces resource updates through a replacement strategy.
	// +optional
	Force bool `json:"force,omitempty"`

	// CleanupOnFail allows deletion of new resources created during the Helm
	// rollback action when it fails.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

// Uninstall holds the configuration for Helm uninstall actions for this
// HelmRelease.
type Uninstall struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like
	// Jobs for hooks) during the performance of a Helm uninstall action. Defaults
	// to 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// KeepHistory tells Helm to remove all associated resources and mark the
	// release as deleted, but retain the release history.
	// +optional
	KeepHistory bool `json:"keepHistory,omitempty"`

	// DisableWait disables waiting for all the resources to be deleted after
	// a Helm uninstall is performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DeletionPropagation specifies the deletion propagation policy when
	// a Helm uninstall is performed.
	// +kubebuilder:default=background
	// +kubebuilder:validation:Enum=background;foreground;orphan
	// +optional
	DeletionPropagation *string `json:"deletionPropagation,omitempty"`
}

// Kustomize Helm PostRenderer specification.
type Kustomize struct {
	// Strategic merge patches, defined as inline YAML objects.
	// +optional
	PatchesStrategicMerge []apiextensionsv1.JSON `json:"patchesStrategicMerge,omitempty"`

	// JSON 6902 patches, defined as inline YAML objects.
	// +optional
	PatchesJSON6902 []kustomize.JSON6902Patch `json:"patchesJson6902,omitempty"`

	// Images is a list of (image name, new name, new tag or digest)
	// for changing image names, tags or digests. This can also be achieved with a
	// patch, but this operator is simpler to specify.
	// +optional
	Images []kustomize.Image `json:"images,omitempty" yaml:"images,omitempty"`
}

// PostRenderer contains a Helm PostRenderer specification.
type PostRenderer struct {
	// Kustomization to apply as PostRenderer.
	// +optional
	Kustomize *Kustomize `json:"kustomize,omitempty"`
}
```

### Reference types

```go
// CrossNamespaceObjectReference contains enough information to let you locate the
// typed referenced object at cluster level.
type CrossNamespaceObjectReference struct {
	// APIVersion of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent.
	// +kubebuilder:validation:Enum=HelmRepository
	// +required
	Kind string `json:"kind,omitempty"`

	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Namespace of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ValuesReference contains a reference to a resource containing Helm values,
// and optionally the key they can be found at.
type ValuesReference struct {
	// Kind of the values referent, valid values are ('Secret', 'ConfigMap').
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind"`

	// Name of the values referent. Should reside in the same namespace as the
	// referring resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// ValuesKey is the data key where the values.yaml or a specific value can be
	// found at. Defaults to 'values.yaml'.
	// +optional
	ValuesKey string `json:"valuesKey,omitempty"`

	// TargetPath is the YAML dot notation path the value should be merged at. When
	// set, the ValuesKey is expected to be a single flat value. Defaults to 'None',
	// which results in the values getting merged at the root.
	// +optional
	TargetPath string `json:"targetPath,omitempty"`

	// Optional marks this ValuesReference as optional. When set, a not found error
	// for the values reference is ignored, but any ValuesKey, TargetPath or
	// transient error will still result in a reconciliation failure.
	// +optional
	Optional bool `json:"optional,omitempty"`
}
```

### Status specification

```go
// HelmReleaseStatus defines the observed state of a HelmRelease.
type HelmReleaseStatus struct {
	// ObservedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastHandledReconcileAt is the last manual reconciliation request (by
	// annotating the HelmRelease) handled by the reconciler.
	// +optional
	LastHandledReconcileAt string `json:"lastHandledReconcileAt,omitempty"`

	// Conditions holds the conditions for the HelmRelease.
	// +optional
	Conditions []meta.Condition `json:"conditions,omitempty"`

	// LastAppliedRevision is the revision of the last successfully applied source.
	// +optional
	LastAppliedRevision string `json:"lastAppliedRevision,omitempty"`

	// LastAttemptedRevision is the revision of the last reconciliation attempt.
	// +optional
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`

	// LastAttemptedValuesChecksum is the SHA1 checksum of the values of the last
	// reconciliation attempt.
	// +optional
	LastAttemptedValuesChecksum string `json:"lastAttemptedValuesChecksum,omitempty"`

	// LastReleaseRevision is the revision of the last successful Helm release.
	// +optional
	LastReleaseRevision int `json:"lastReleaseRevision,omitempty"`

	// HelmChart is the namespaced name of the HelmChart resource created by
	// the controller for the HelmRelease.
	// +optional
	HelmChart string `json:"helmChart,omitempty"`

	// Failures is the reconciliation failure count against the latest observed
	// state. It is reset after a successful reconciliation.
	// +optional
	Failures int64 `json:"failures,omitempty"`

	// InstallFailures is the install failure count against the latest observed
	// state. It is reset after a successful reconciliation.
	// +optional
	InstallFailures int64 `json:"installFailures,omitempty"`

	// UpgradeFailures is the upgrade failure count against the latest observed
	// state. It is reset after a successful reconciliation.
	// +optional
	UpgradeFailures int64 `json:"upgradeFailures,omitempty"`
}
```

#### Condition types

```go
const (
	// ReleasedCondition represents the status of the last release attempt
	// (install/upgrade/test) against the latest desired state.
	ReleasedCondition string = "Released"

	// TestSuccessCondition represents the status of the last test attempt against
	// the latest desired state.
	TestSuccessCondition string = "TestSuccess"

	// RemediatedCondition represents the status of the last remediation attempt
	// (uninstall/rollback) due to a failure of the last release attempt against the
	// latest desired state.
	RemediatedCondition string = "Remediated"
)
```

#### Condition reasons

```go
const (
	// InstallSucceededReason represents the fact that the Helm install for the
	// HelmRelease succeeded.
	InstallSucceededReason string = "InstallSucceeded"

	// InstallFailedReason represents the fact that the Helm install for the
	// HelmRelease failed.
	InstallFailedReason string = "InstallFailed"

	// UpgradeSucceededReason represents the fact that the Helm upgrade for the
	// HelmRelease succeeded.
	UpgradeSucceededReason string = "UpgradeSucceeded"

	// UpgradeFailedReason represents the fact that the Helm upgrade for the
	// HelmRelease failed.
	UpgradeFailedReason string = "UpgradeFailed"

	// TestSucceededReason represents the fact that the Helm tests for the
	// HelmRelease succeeded.
	TestSucceededReason string = "TestSucceeded"

	// TestFailedReason represents the fact that the Helm tests for the HelmRelease
	// failed.
	TestFailedReason string = "TestFailed"

	// RollbackSucceededReason represents the fact that the Helm rollback for the
	// HelmRelease succeeded.
	RollbackSucceededReason string = "RollbackSucceeded"

	// RollbackFailedReason represents the fact that the Helm test for the
	// HelmRelease failed.
	RollbackFailedReason string = "RollbackFailed"

	// UninstallSucceededReason represents the fact that the Helm uninstall for the
	// HelmRelease succeeded.
	UninstallSucceededReason string = "UninstallSucceeded"

	// UninstallFailedReason represents the fact that the Helm uninstall for the
	// HelmRelease failed.
	UninstallFailedReason string = "UninstallFailed"

	// ArtifactFailedReason represents the fact that the artifact download for the
	// HelmRelease failed.
	ArtifactFailedReason string = "ArtifactFailed"

	// InitFailedReason represents the fact that the initialization of the Helm
	// configuration failed.
	InitFailedReason string = "InitFailed"

	// GetLastReleaseFailedReason represents the fact that observing the last
	// release failed.
	GetLastReleaseFailedReason string = "GetLastReleaseFailed"
)
```

## Helm release placement

The namespace/name in which to deploy and store the Helm release defaults to the namespace/name
of the `HelmRelease`. These can be overridden respectively via `spec.targetNamespace`,
`spec.storageNamespace` and `spec.releaseName`. If `spec.targetNamespace` is set,
`spec.releaseName` defaults to `<spec.targetNamespace>-<metadata.name>`.

> **Note:** that configuring the `spec.targetNamespace` only defines the namespace the release
> is made in, the metadata for the release (also known as the "Helm storage") will be stored in
> the `metadata.namespace` or `spec.storageNamespace` of the `HelmRelease`.

## Helm chart template

The `spec.chart` values are used by the helm-controller as a template
to create a new `HelmChart` resource with the given spec.

The `spec.chart.spec.sourceRef` is a reference to an object managed by
[source-controller](https://github.com/fluxcd/source-controller). When the source
[revision](https://pkg.go.dev/github.com/fluxcd/source-controller/api/v1beta2#Artifact)
changes, it generates a Kubernetes event that triggers a new release.

Supported source types:

- [HelmRepository](https://github.com/fluxcd/source-controller/blob/main/docs/spec/v1beta2/helmrepositories.md)
- [GitRepository](https://github.com/fluxcd/source-controller/blob/main/docs/spec/v1beta2/gitrepositories.md)
- [Bucket](https://github.com/fluxcd/source-controller/blob/main/docs/spec/v1beta2/buckets.md)

The `HelmChart` is created in the same namespace as the `sourceRef`,
with a name matching the `HelmRelease` `<metadata.namespace>-<metadata.name>`.

> **Note** that on multi-tenant clusters,  platform admins can disable cross-namespace references
> with the `--no-cross-namespace-refs=true` flag. When this flag is set, the HelmRelease can only
> refer to sources in the same  namespace as the HelmRelease object.

The `chart.spec.chart` can either contain:

- The name of the chart as made available by the `HelmRepository`
  (without any aliases), for example: `podinfo`
- The relative path the chart can be found at in the `GitRepository`,
  for example: `./charts/podinfo`

The `chart.spec.version` can be a fixed semver, or any semver range
(i.e. `>=4.0.0 <5.0.0`). It is ignored for `HelmRelease` resources
that reference a `GitRepository` or `Bucket` source.

Annotations and labels can be added by configuring the respective `.spec.chart.metadata`
fields.

## Values overrides

The simplest way to define values overrides is inline via `spec.values`.
It is also possible to define a list of `ConfigMap` and `Secret` resources
from which to take values via `spec.valuesFrom`. The values are merged in the order given,
with the later values overwriting earlier, and then `spec.values` overwriting those:

```yaml
spec:
  values:
    replicaCount: 2
  valuesFrom:
    - kind: ConfigMap
      name: prod-env-values
      valuesKey: values-prod.yaml
    - kind: Secret
      name: prod-tls-values
      valuesKey: crt
      targetPath: tls.crt
      optional: true
```

The definition of the listed keys for items in `spec.valuesFrom` is as follows:

- `kind`: Kind of the values referent (`ConfigMap` or `Secret`).
- `name`: Name of the values referent, in the same namespace as the
  `HelmRelease`.
- `valuesKey` _(Optional)_: The data key where the values.yaml or a
  specific value can be found. Defaults to `values.yaml` when omitted.
- `targetPath` _(Optional)_: The YAML dot notation path at which the
  value should be merged. When set, the `valuesKey` is expected to be
  a single flat value. Defaults to `None` when omitted, which results
  in the values getting merged at the root.
- `optional` _(Optional)_: Whether this values reference is optional. When `true`,
  a not found error for the values reference is ignored, but any valuesKey, targetPath or
  transient error will still result in a reconciliation failure. Defaults to `false`
  when omitted.

> **Note:** that the `targetPath` supports the same formatting as you would supply as an
> argument to the `helm` binary using `--set [path]=[value]`. In addition to this, the
> referred value can contain the same value formats (e.g. `{a,b,c}` for a list). You can
> read more about the available formats and limitations in the
> [Helm documentation](https://helm.sh/docs/intro/using_helm/#the-format-and-limitations-of---set).

## Reconciliation

If no Helm release with the matching namespace/name is found it will be installed. It will
be upgraded any time the desired state is updated, which consists of:

- `spec` (and thus `metadata.generation`)
- Latest `HelmChart` revision available
- [`ConfigMap` and `Secret` values overrides](#values-overrides). Changes to these do not trigger an
  immediate reconciliation, but will be handled upon the next reconciliation. This is to avoid
  a large number of upgrades occurring when multiple resources are updated.

If the latest Helm release revision was not made by the helm-controller, it may not match the
desired state, so an upgrade is made in this case as well.

The `spec.interval` tells the reconciler at which interval to reconcile the release. The
interval time units are `s`, `m` and `h` e.g. `interval: 5m`, the minimum value should be 60 seconds.

**Note:** The controller can be configured to apply a jitter to the interval in
order to distribute the load more evenly when multiple HelmRelease objects are
set up with the same interval. For more information, please refer to the
[helm-controller configuration options](https://fluxcd.io/flux/components/helm/options/).

The reconciler can be told to reconcile the `HelmRelease` outside of the specified interval
by annotating the object with a `reconcile.fluxcd.io/requestedAt` annotation. For example:

```bash
kubectl annotate --overwrite helmrelease/podinfo reconcile.fluxcd.io/requestedAt="$(date +%s)"
```

Reconciliation can be suspended by setting `spec.suspend` to `true`.

The timeout for any individual Kubernetes operation (like Jobs for hooks) during the performance
of Helm actions can be configured via `spec.timeout` and can be overridden per action
via `spec.<action>.timeout`.

List all Kubernetes objects reconciled from a HelmRelease:

```sh
kubectl get all --all-namespaces \
-l=helm.toolkit.fluxcd.io/name="<HelmRelease name>" \
-l=helm.toolkit.fluxcd.io/namespace="<HelmRelease namespace>"
```

### Disabling resource waiting

For install, upgrade, rollback, and uninstall actions resource waiting is
enabled by default, but can be disabled by setting `spec.<action>.disableWait`.

Waiting for jobs to complete is enabled by default, 
but can be disabled by setting `spec.<action>.disableWaitForJobs`.

### `HelmRelease` dependencies

When applying a `HelmRelease`, you may need to make sure other releases are [Ready](#status)
before the release is reconciled. For example, because your chart relies on the presence of
a Custom Resource Definition installed by another `HelmRelease`. The `spec.dependsOn` field
allows you to specify each of these dependencies.

Assuming two `HelmRelease` resources:

- `backend` - contains the backend of the application
- `frontend` - contains the frontend of the application and relies on the backend

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: backend
  namespace: default
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: podinfo
        namespace: default
      interval: 1m
  upgrade:
    remediation:
      remediateLastFailure: true
  test:
    enable: true
  values:
    service:
      grpcService: backend
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: frontend
  namespace: default
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: podinfo
        namespace: default
      interval: 1m
  dependsOn:
    - name: backend
  upgrade:
    remediation:
      remediateLastFailure: true
  test:
    enable: true
  values:
    backend: http://backend-podinfo:9898/echo
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

> **Note** that this does not account for upgrade ordering. Kubernetes only allows
> applying one resource (`HelmRelease` in this case) at a time, so there is no way for the
> controller to know when a dependency `HelmRelease` may be updated. Also, circular
> dependencies between `HelmRelease` resources must be avoided, otherwise the
> interdependent `HelmRelease` resources will never be reconciled.

### Configuring Helm test actions

To make the controller run the Helm tests available for your chart after a successful Helm install
or upgrade, `spec.test.enable` should be set to `true`.

By default, when tests are enabled, failures in tests are considered release failures, and thus
are subject to the triggering Helm action's `remediation` configuration. However, test failures
can be ignored by setting `spec.test.ignoreFailures` to `true`. In this case, no remediation will
be taken, and the test failure will not affect the `Released` and `Ready` status conditions. This
can be overridden per Helm action by setting `spec.install.remediation.ignoreTestFailures`
or `spec.upgrade.remediation.ignoreTestFailures`.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: podinfo
        namespace: default
      interval: 1m
  test:
    enable: true
    ignoreFailures: true
  values:
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

### Configuring failure remediation

From time to time a Helm install/upgrade and accompanying [Helm test](#configuring-helm-test-actions)
may fail. When this occurs, by default no action is taken, and the release is left in a failed state.
However, several automatic failure remediation options can be set via
`spec.install.remediation` and `spec.upgrade.remediation`.

The `retries` can be set to configure the number of retries after an initial
failure. A negative integer results in infinite retries. This implicitly opts-in to a remediation
action between each attempt. The remediation action for install failures is an uninstall. The
remediation action for upgrade failures is by default a rollback, however
`spec.upgrade.remediation.strategy` can be set to `uninstall`, in which case after the uninstall,
the `spec.install` configuration takes over.

One can also opt-in to remediation of the last failure (when no retries remain) by setting
`spec.<action>.remediation.remediateLastFailure` to `true`. For upgrades, this defaults
to true if at least one retry is configured.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: podinfo
        namespace: default
      interval: 1m
  install:
    remediation:
      retries: 3
  upgrade:
    remediation:
      remediateLastFailure: false
  values:
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

## Role-based access control

By default, a `HelmRelease` runs under the cluster admin account and can create, modify, delete cluster level objects
(cluster roles, cluster role binding, CRDs, etc) and namespaced objects (deployments, ingresses, etc).
For certain `HelmReleases` a cluster admin may wish to control what types of Kubernetes objects can
be reconciled and under which namespace.
To restrict a `HelmRelease`, one can assign a service account under which the reconciliation is performed.

Assuming you want to restrict a group of `HelmReleases` to a single namespace, you can create an account
with a role binding that grants access only to that namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: webapp
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: webapp-reconciler
  namespace: webapp
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: webapp-reconciler
  namespace: webapp
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: webapp-reconciler
  namespace: webapp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: webapp-reconciler
subjects:
  - kind: ServiceAccount
    name: webapp-reconciler
    namespace: webapp
```

> **Note** that the namespace, RBAC and service account manifests should be
> placed in a Git source and applied with a Kustomization.

Create a `HelmRelease` that prevents altering the cluster state outside of the `webapp` namespace:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: podinfo
  namespace: webapp
spec:
  serviceAccountName: webapp-reconciler
  interval: 5m
  chart:
    spec:
      chart: podinfo
      sourceRef:
        kind: HelmRepository
        name: podinfo
```

When the controller reconciles the `podinfo` release, it will impersonate the `webapp-reconciler`
account. If the chart contains cluster level objects like CRDs, the reconciliation will fail since
the account it runs under has no permissions to alter objects outside of the `webapp` namespace.

### Enforce impersonation

On multi-tenant clusters, platform admins can enforce impersonation with the
`--default-service-account` flag.

When the flag is set, all HelmReleases which don't have `spec.serviceAccountName` specified
will use the service account name provided by `--default-service-account=<SA Name>`
in the namespace of the object.

## Remote Clusters / Cluster-API

If the `spec.kubeConfig` field is set, Helm actions will run against the default cluster specified
in that KubeConfig instead of the local cluster that is responsible for the reconciliation of the
HelmRelease.

The secret defined in the `spec.kubeConfig.secretRef` must exist in the same namespace as the
HelmRelease. On every reconciliation, the KubeConfig bytes will be loaded from the `.secretRef.key`
key (default: `value` or `value.yaml`) of the Secret's data , and the Secret can thus be regularly
updated if cluster-access-tokens have to rotate due to expiration.

The Helm storage is stored on the remote cluster in a namespace that equals to the namespace of
the HelmRelease, or the configured `spec.storageNamespace`. The release itself is made in a
namespace that equals to the namespace of the HelmRelease, or the configured `spec.targetNamespace`.
The namespaces are expected to exist, with the exception that `spec.targetNamespace` can be
created on demand by Helm when `spec.install.createNamespace` is set to `true`.

Other references to Kubernetes resources in the HelmRelease, like ValuesReference resources,
are expected to exist on the reconciling cluster.

This composes well with Cluster API bootstrap providers such as CAPBK (kubeadm), as well as the
CAPA (AWS) EKS integration.

To reconcile a HelmRelease to a CAPI controlled cluster, put the HelmRelease in the same
namespace as your Cluster object, and set the `spec.kubeConfig.secretRef.name` to
`<cluster-name>-kubeconfig`:

```yaml
apiVersion: cluster.x-k8s.io/v1alpha3
kind: Cluster
metadata:
  name: stage # the kubeconfig Secret will contain the Cluster name
  namespace: capi-stage
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
        - 10.100.0.0/16
    serviceDomain: stage-cluster.local
    services:
      cidrBlocks:
        - 10.200.0.0/12
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1alpha3
    kind: KubeadmControlPlane
    name: stage-control-plane
    namespace: capi-stage
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha3
    kind: DockerCluster
    name: stage
    namespace: capi-stage
---
# ... unrelated Cluster API objects omitted for brevity ...
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: kube-prometheus-stack
  namespace: capi-stage
spec:
  kubeConfig:
    secretRef:
      name: stage-kubeconfig # Cluster API creates this for the matching Cluster
  chart:
    spec:
      chart: prometheus
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: prometheus-community
  install:
    remediation:
      retries: -1
```

The Cluster and HelmRelease can be created at the same time if the install remediation
configuration is set to a forgiving amount of retries. The HelmRelease will then eventually
reconcile once the cluster is available.

If you wish to target clusters created by other means than CAPI, you can create a ServiceAccount
on the remote cluster, generate a KubeConfig for that account, and then create a secret on the
cluster where helm-controller is running e.g.:

```sh
kubectl -n default create secret generic prod-kubeconfig \
    --from-file=value.yaml=./kubeconfig
```

> **Note** that the KubeConfig should be self-contained and not rely on binaries, environment,
> or credential files from the helm-controller Pod. This matches the constraints of KubeConfigs
> from current Cluster API providers. KubeConfigs with cmd-path in them likely won't work without
> a custom, per-provider installation of helm-controller.

When both `spec.kubeConfig` and `spec.ServiceAccountName` are specified,
the controller will impersonate the service account on the target cluster.

## Post Renderers

HelmRelease resources has a built-in [Kustomize](https://kubectl.docs.kubernetes.io/references/kustomize/builtins/)
compatible [Post Renderer](https://helm.sh/docs/topics/advanced/#post-rendering), which provides
the following Kustomize directives:

- [patches](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/patches/)
- [patchesStrategicMerge](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/patchesstrategicmerge/)
- [patchesJson6902](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/patchesjson6902/)
- [images](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/images/)

The following example uses the built-in `kustomize` _Post Renderer_ to apply a strategic merge patch,
which adds a [toleration](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/)
to the Helm rendered output:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: metrics-server
  namespace: kube-system
spec:
  interval: 1m
  chart:
    spec:
      chart: metrics-server
      version: "5.3.4"
      sourceRef:
        kind: HelmRepository
        name: bitnami
        namespace: kube-system
      interval: 1m
  postRenderers:
    # Instruct helm-controller to use built-in "kustomize" post renderer.
    - kustomize:
        # Array of inline strategic merge patch definitions as YAML object.
        # Note, this is a YAML object and not a string, to avoid syntax
        # indention errors.
        patchesStrategicMerge:
          - kind: Deployment
            apiVersion: apps/v1
            metadata:
              name: metrics-server
            spec:
              template:
                spec:
                  tolerations:
                    - key: "workload-type"
                      operator: "Equal"
                      value: "cluster-services"
                      effect: "NoSchedule"
        # Array of inline JSON6902 patch definitions as YAML object.
        # Note, this is a YAML object and not a string, to avoid syntax
        # indention errors.
        patchesJson6902:
          - target:
              version: v1
              kind: Deployment
              name: metrics-server
            patch:
              - op: add
                path: /spec/template/priorityClassName
                value: system-cluster-critical
        images:
          - name: docker.io/bitnami/metrics-server
            newName: docker.io/bitnami/metrics-server
            newTag: 0.4.1-debian-10-r54
```

## CRDs

Helm does support [installing CRDs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you),
but it has no native support for [upgrading CRDs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#some-caveats-and-explanations):

> There is no support at this time for upgrading or deleting CRDs using Helm.
> This was an explicit decision after much community discussion due to the danger for unintentional
> data loss. Furthermore, there is currently no community consensus around how to handle CRDs and
> their lifecycle. As this evolves, Helm will add support for those use cases.

If your write your own Helm Charts you can work-around this limitation by putting your CRDs into
the `templates` instead of the `crds` directory or by out-factoring them into a separate Helm
Chart as suggested by the
[offical Helm documentation](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-2-separate-charts).

However, if you have to integrate and use many existing (upstream) Helm Charts, not being able to
upgrade the CRDs via Flux's `HelmRelease` objects might become a cumbersome limitation within your GitOps
workflow. Therefore, Flux allows you to opt-in to upgrading CRDs by setting the `crds` policy on
the `HelmRelease.spec.install` and `HelmRelease.spec.upgrade` objects.

The following CRD upgrade policies are supported:

- `Skip` Skip CRDs do neither install nor replace (update) any CRDs.
- `Create` Only create new CRDs which do not yet exist, neither update nor delete any existing CRDs.
- `CreateReplace` Create new CRDs, update (replace) existing ones, but do **not** delete CRDs which
  no longer exist in the current helm release.

**Example**:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: my-operator
  namespace: default
spec:
  interval: 1m
  chart:
    spec:
      chart: my-operator
      version: "1.0.1"
      sourceRef:
        kind: HelmRepository
        name: my-operator-repo
        namespace: default
      interval: 1m
  install:
    crds: CreateReplace
  upgrade:
    crds: CreateReplace
```

## Drift detection

**Note:** This feature is experimental and can be enabled by setting `--feature-gates=DetectDrift=true`.

When a HelmRelease is in-sync with the Helm release object in the storage, the controller will
compare the manifests from the Helm storage with the current state of the cluster using a
[server-side dry-run apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).
If this comparison detects a drift (either due resource being created or modified during the
dry-run), the controller will perform an upgrade for the release, restoring the desired state.

To help aid transition to this new feature, it is possible to enable drift detection without it
correcting drift. This can be done by adding `CorrectDrift=false` to the `--feature-gates` flag,
i.e. `--feature-gates=DetectDrift=true,CorrectDrift=false`. This will allow you to see what drift
is detected in the controller logs (with `--log-level=debug`), to potentially add the appropriate
[exclusions annotations or labels](#excluding-resources-from-drift-detection), before enabling the
feature full.

### Excluding resources from drift detection

The drift detection feature can be configured to exclude certain resources from the comparison
by labeling or annotating them with `helm.toolkit.fluxcd.io/driftDetection: disabled`. Using
[post-renderers](#post-renderers), this can be applied to any resource rendered by Helm.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: app
  namespace: webapp
spec:
  postRenderers:
    - kustomize:
        patches:
          - target:
              version: v1
              kind: Deployment
              name: my-app
            patch: |
              - op: add
                path: /metadata/annotations/helm.toolkit.fluxcd.io~1driftDetection
                value: disabled
```

**Note:** For some charts, we have observed the drift detection feature can detect spurious
changes due to Helm not properly patching an object, which seems to be related to
[Helm#5915](https://github.com/helm/helm/issues/5915) and issues alike. In this case (and
when possible for your workload), configuring `.spec.upgrade.force` to `true` might be a
more fitting solution than ignoring the object in full.

#### Drift exclusion example Prometheus Stack

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: kube-prometheus-stack
spec:
  interval: 5m
  chart:
    spec:
      version: "45.x"
      chart: kube-prometheus-stack
      sourceRef:
        kind: HelmRepository
        name: prometheus-community
      interval: 60m
  upgrade:
    crds: CreateReplace
    # Force recreation due to Helm not properly patching Deployment with e.g. added port,
    # causing spurious drift detection
    force: true
  postRenderers:
    - kustomize:
        patches:
          - target:
              # Ignore these objects from Flux diff as they are mutated from chart hooks
              kind: (ValidatingWebhookConfiguration|MutatingWebhookConfiguration)
              name: kube-prometheus-stack-admission
            patch: |
              - op: add
                path: /metadata/annotations/helm.toolkit.fluxcd.io~1driftDetection
                value: disabled
          - target:
              # Ignore these objects from Flux diff as they are mutated at apply time but not
              # at dry-run time
              kind: PrometheusRule
            patch: |
              - op: add
                path: /metadata/annotations/helm.toolkit.fluxcd.io~1driftDetection
                value: disabled
```

## Status

When the controller completes a reconciliation, it reports the result in the status sub-resource.

The following `status.conditions` types are advertised. Here, "desired state" is as detailed in
[reconciliation](#reconciliation):

- `Ready` - status of the last reconciliation attempt
- `Released` - status of the last release attempt (install/upgrade/test) against the latest desired state
- `TestSuccess` - status of the last test attempt against the latest desired state
- `Remediated` - status of the last remediation attempt (uninstall/rollback) due to a failure of the
  last release attempt against the latest desired state

For example, you can wait for a successful helm-controller reconciliation with:

```sh
kubectl wait helmrelease/podinfo --for=condition=ready
```

Each of these conditions also include descriptive `reason` / `message` fields
as to why the status is as such.

### Examples

#### Install success

```yaml
status:
  conditions:
    - lastTransitionTime: "2020-07-13T13:13:40Z"
      message: Helm install succeeded
      reason: InstallSucceeded
      status: "True"
      type: Released
    - lastTransitionTime: "2020-07-13T13:13:40Z"
      message: Helm test succeeded
      reason: TestSucceeded
      status: "True"
      type: TestSuccess
    - lastTransitionTime: "2020-07-13T13:13:42Z"
      message: release reconciliation succeeded
      reason: ReconciliationSucceeded
      status: "True"
      type: Ready
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastReleaseRevision: 1
  observedGeneration: 2
```

#### Upgrade failure

```yaml
status:
  conditions:
    - lastTransitionTime: "2020-07-13T13:17:28Z"
      message:
        'error validating "": error validating data: ValidationError(Deployment.spec.replicas):
        invalid type for io.k8s.api.apps.v1.DeploymentSpec.replicas: got "string",
        expected "integer"'
      reason: UpgradeFailed
      status: "False"
      type: Released
    - lastTransitionTime: "2020-07-13T13:17:28Z"
      message:
        'error validating "": error validating data: ValidationError(Deployment.spec.replicas):
        invalid type for io.k8s.api.apps.v1.DeploymentSpec.replicas: got "string",
        expected "integer"'
      reason: UpgradeFailed
      status: "False"
      type: Ready
  failures: 1
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastReleaseRevision: 1
  observedGeneration: 3
```

#### Ignored test failure

```yaml
status:
  conditions:
    - lastTransitionTime: "2020-07-13T13:13:40Z"
      message: Helm install succeeded
      reason: InstallSucceeded
      status: "True"
      type: Released
    - lastTransitionTime: "2020-07-13T13:13:40Z"
      message: Helm test failed
      reason: TestFailed
      status: "False"
      type: TestSuccess
    - lastTransitionTime: "2020-07-13T13:13:42Z"
      message: release reconciliation succeeded
      reason: ReconciliationSucceeded
      status: "True"
      type: Ready
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastReleaseRevision: 1
  observedGeneration: 2
```
