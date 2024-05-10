<h1>Helm API reference v2</h1>
<p>Packages:</p>
<ul class="simple">
<li>
<a href="#helm.toolkit.fluxcd.io%2fv2">helm.toolkit.fluxcd.io/v2</a>
</li>
</ul>
<h2 id="helm.toolkit.fluxcd.io/v2">helm.toolkit.fluxcd.io/v2</h2>
<p>Package v2 contains API Schema definitions for the helm v2 API group</p>
Resource Types:
<ul class="simple"><li>
<a href="#helm.toolkit.fluxcd.io/v2.HelmRelease">HelmRelease</a>
</li></ul>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmRelease">HelmRelease
</h3>
<p>HelmRelease is the Schema for the helmreleases API</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br>
string</td>
<td>
<code>helm.toolkit.fluxcd.io/v2</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br>
string
</td>
<td>
<code>HelmRelease</code>
</td>
</tr>
<tr>
<td>
<code>metadata</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">
HelmReleaseSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>chart</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplate">
HelmChartTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Chart defines the template of the v1.HelmChart that should be created
for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>chartRef</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.CrossNamespaceSourceReference">
CrossNamespaceSourceReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ChartRef holds a reference to a source controller resource containing the
Helm chart artifact.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<p>Interval at which to reconcile the Helm release.</p>
</td>
</tr>
<tr>
<td>
<code>kubeConfig</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#KubeConfigReference">
github.com/fluxcd/pkg/apis/meta.KubeConfigReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KubeConfig for reconciling the HelmRelease on a remote cluster.
When used in combination with HelmReleaseSpec.ServiceAccountName,
forces the controller to act on behalf of that Service Account at the
target cluster.
If the &ndash;default-service-account flag is set, its value will be used as
a controller level fallback for when HelmReleaseSpec.ServiceAccountName
is empty.</p>
</td>
</tr>
<tr>
<td>
<code>suspend</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Suspend tells the controller to suspend reconciliation for this HelmRelease,
it does not apply to already started reconciliations. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>releaseName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReleaseName used for the Helm release. Defaults to a composition of
&lsquo;[TargetNamespace-]Name&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>targetNamespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetNamespace to target when performing operations for the HelmRelease.
Defaults to the namespace of the HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>storageNamespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StorageNamespace used for the Helm storage.
Defaults to the namespace of the HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>dependsOn</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#NamespacedObjectReference">
[]github.com/fluxcd/pkg/apis/meta.NamespacedObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DependsOn may contain a meta.NamespacedObjectReference slice with
references to HelmRelease resources that must be ready before this HelmRelease
can be reconciled.</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation (like Jobs
for hooks) during the performance of a Helm action. Defaults to &lsquo;5m0s&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>maxHistory</code><br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxHistory is the number of revisions saved by Helm for this HelmRelease.
Use &lsquo;0&rsquo; for an unlimited number of revisions; defaults to &lsquo;5&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the Kubernetes service account to impersonate
when reconciling this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>persistentClient</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>PersistentClient tells the controller to use a persistent Kubernetes
client for this release. When enabled, the client will be reused for the
duration of the reconciliation, instead of being created and destroyed
for each (step of a) Helm action.</p>
<p>This can improve performance, but may cause issues with some Helm charts
that for example do create Custom Resource Definitions during installation
outside Helm&rsquo;s CRD lifecycle hooks, which are then not observed to be
available by e.g. post-install hooks.</p>
<p>If not set, it defaults to true.</p>
</td>
</tr>
<tr>
<td>
<code>driftDetection</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.DriftDetection">
DriftDetection
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DriftDetection holds the configuration for detecting and handling
differences between the manifest in the Helm storage and the resources
currently existing in the cluster.</p>
</td>
</tr>
<tr>
<td>
<code>install</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Install">
Install
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Install holds the configuration for Helm install actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>upgrade</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Upgrade">
Upgrade
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Upgrade holds the configuration for Helm upgrade actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>test</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Test">
Test
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Test holds the configuration for Helm test actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>rollback</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Rollback">
Rollback
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Rollback holds the configuration for Helm rollback actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>uninstall</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Uninstall">
Uninstall
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Uninstall holds the configuration for Helm uninstall actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>valuesFrom</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.ValuesReference">
[]ValuesReference
</a>
</em>
</td>
<td>
<p>ValuesFrom holds references to resources containing Helm values for this HelmRelease,
and information about how they should be merged.</p>
</td>
</tr>
<tr>
<td>
<code>values</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Values holds the values for this Helm release.</p>
</td>
</tr>
<tr>
<td>
<code>postRenderers</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.PostRenderer">
[]PostRenderer
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PostRenderers holds an array of Helm PostRenderers, which will be applied in order
of their definition.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseStatus">
HelmReleaseStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.CRDsPolicy">CRDsPolicy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.Install">Install</a>, 
<a href="#helm.toolkit.fluxcd.io/v2.Upgrade">Upgrade</a>)
</p>
<p>CRDsPolicy defines the install/upgrade approach to use for CRDs when
installing or upgrading a HelmRelease.</p>
<h3 id="helm.toolkit.fluxcd.io/v2.CrossNamespaceObjectReference">CrossNamespaceObjectReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplateSpec">HelmChartTemplateSpec</a>)
</p>
<p>CrossNamespaceObjectReference contains enough information to let you locate
the typed referenced object at cluster level.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>APIVersion of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Namespace of the referent.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.CrossNamespaceSourceReference">CrossNamespaceSourceReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>CrossNamespaceSourceReference contains enough information to let you locate
the typed referenced object at cluster level.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>APIVersion of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Namespace of the referent, defaults to the namespace of the Kubernetes
resource object that contains the reference.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.DriftDetection">DriftDetection
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>DriftDetection defines the strategy for performing differential analysis and
provides a way to define rules for ignoring specific changes during this
process.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>mode</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.DriftDetectionMode">
DriftDetectionMode
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mode defines how differences should be handled between the Helm manifest
and the manifest currently applied to the cluster.
If not explicitly set, it defaults to DiffModeDisabled.</p>
</td>
</tr>
<tr>
<td>
<code>ignore</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.IgnoreRule">
[]IgnoreRule
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Ignore contains a list of rules for specifying which changes to ignore
during diffing.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.DriftDetectionMode">DriftDetectionMode
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.DriftDetection">DriftDetection</a>)
</p>
<p>DriftDetectionMode represents the modes in which a controller can detect and
handle differences between the manifest in the Helm storage and the resources
currently existing in the cluster.</p>
<h3 id="helm.toolkit.fluxcd.io/v2.Filter">Filter
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.Test">Test</a>)
</p>
<p>Filter holds the configuration for individual Helm test filters.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the test.</p>
</td>
</tr>
<tr>
<td>
<code>exclude</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Exclude specifies whether the named test should be excluded.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmChartTemplate">HelmChartTemplate
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>HelmChartTemplate defines the template from which the controller will
generate a v1.HelmChart object in the same namespace as the referenced
v1.Source.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplateObjectMeta">
HelmChartTemplateObjectMeta
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObjectMeta holds the template for metadata like labels and annotations.</p>
</td>
</tr>
<tr>
<td>
<code>spec</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplateSpec">
HelmChartTemplateSpec
</a>
</em>
</td>
<td>
<p>Spec holds the template for the v1.HelmChartSpec for this HelmRelease.</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>chart</code><br>
<em>
string
</em>
</td>
<td>
<p>The name or path the Helm chart is available at in the SourceRef.</p>
</td>
</tr>
<tr>
<td>
<code>version</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Version semver expression, ignored for charts from v1.GitRepository and
v1beta2.Bucket sources. Defaults to latest when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.CrossNamespaceObjectReference">
CrossNamespaceObjectReference
</a>
</em>
</td>
<td>
<p>The name and namespace of the v1.Source the chart is available at.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Interval at which to check the v1.Source for updates. Defaults to
&lsquo;HelmReleaseSpec.Interval&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>reconcileStrategy</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Determines what enables the creation of a new artifact. Valid values are
(&lsquo;ChartVersion&rsquo;, &lsquo;Revision&rsquo;).
See the documentation of the values for an explanation on their behavior.
Defaults to ChartVersion when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>valuesFiles</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Alternative list of values files to use as the chart values (values.yaml
is not included by default), expected to be a relative path in the SourceRef.
Values files are merged in the order of this list with the last file overriding
the first. Ignored when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>ignoreMissingValuesFiles</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>IgnoreMissingValuesFiles controls whether to silently ignore missing values files rather than failing.</p>
</td>
</tr>
<tr>
<td>
<code>verify</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplateVerification">
HelmChartTemplateVerification
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Verify contains the secret name containing the trusted public keys
used to verify the signature and specifies which provider to use to check
whether OCI image is authentic.
This field is only supported for OCI sources.
Chart dependencies, which are not bundled in the umbrella chart artifact,
are not verified.</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmChartTemplateObjectMeta">HelmChartTemplateObjectMeta
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplate">HelmChartTemplate</a>)
</p>
<p>HelmChartTemplateObjectMeta defines the template for the ObjectMeta of a
v1.HelmChart.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>labels</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Map of string keys and values that can be used to organize and categorize
(scope and select) objects.
More info: <a href="https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/">https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/</a></p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Annotations is an unstructured key value map stored with a resource that may be
set by external tools to store and retrieve arbitrary metadata. They are not
queryable and should be preserved when modifying objects.
More info: <a href="https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/">https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/</a></p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmChartTemplateSpec">HelmChartTemplateSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplate">HelmChartTemplate</a>)
</p>
<p>HelmChartTemplateSpec defines the template from which the controller will
generate a v1.HelmChartSpec object.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>chart</code><br>
<em>
string
</em>
</td>
<td>
<p>The name or path the Helm chart is available at in the SourceRef.</p>
</td>
</tr>
<tr>
<td>
<code>version</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Version semver expression, ignored for charts from v1.GitRepository and
v1beta2.Bucket sources. Defaults to latest when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.CrossNamespaceObjectReference">
CrossNamespaceObjectReference
</a>
</em>
</td>
<td>
<p>The name and namespace of the v1.Source the chart is available at.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Interval at which to check the v1.Source for updates. Defaults to
&lsquo;HelmReleaseSpec.Interval&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>reconcileStrategy</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Determines what enables the creation of a new artifact. Valid values are
(&lsquo;ChartVersion&rsquo;, &lsquo;Revision&rsquo;).
See the documentation of the values for an explanation on their behavior.
Defaults to ChartVersion when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>valuesFiles</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Alternative list of values files to use as the chart values (values.yaml
is not included by default), expected to be a relative path in the SourceRef.
Values files are merged in the order of this list with the last file overriding
the first. Ignored when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>ignoreMissingValuesFiles</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>IgnoreMissingValuesFiles controls whether to silently ignore missing values files rather than failing.</p>
</td>
</tr>
<tr>
<td>
<code>verify</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplateVerification">
HelmChartTemplateVerification
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Verify contains the secret name containing the trusted public keys
used to verify the signature and specifies which provider to use to check
whether OCI image is authentic.
This field is only supported for OCI sources.
Chart dependencies, which are not bundled in the umbrella chart artifact,
are not verified.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmChartTemplateVerification">HelmChartTemplateVerification
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplateSpec">HelmChartTemplateSpec</a>)
</p>
<p>HelmChartTemplateVerification verifies the authenticity of an OCI Helm chart.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>provider</code><br>
<em>
string
</em>
</td>
<td>
<p>Provider specifies the technology used to sign the OCI Helm chart.</p>
</td>
</tr>
<tr>
<td>
<code>secretRef</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#LocalObjectReference">
github.com/fluxcd/pkg/apis/meta.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecretRef specifies the Kubernetes Secret containing the
trusted public keys.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmRelease">HelmRelease</a>)
</p>
<p>HelmReleaseSpec defines the desired state of a Helm release.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>chart</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmChartTemplate">
HelmChartTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Chart defines the template of the v1.HelmChart that should be created
for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>chartRef</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.CrossNamespaceSourceReference">
CrossNamespaceSourceReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ChartRef holds a reference to a source controller resource containing the
Helm chart artifact.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<p>Interval at which to reconcile the Helm release.</p>
</td>
</tr>
<tr>
<td>
<code>kubeConfig</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#KubeConfigReference">
github.com/fluxcd/pkg/apis/meta.KubeConfigReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KubeConfig for reconciling the HelmRelease on a remote cluster.
When used in combination with HelmReleaseSpec.ServiceAccountName,
forces the controller to act on behalf of that Service Account at the
target cluster.
If the &ndash;default-service-account flag is set, its value will be used as
a controller level fallback for when HelmReleaseSpec.ServiceAccountName
is empty.</p>
</td>
</tr>
<tr>
<td>
<code>suspend</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Suspend tells the controller to suspend reconciliation for this HelmRelease,
it does not apply to already started reconciliations. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>releaseName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReleaseName used for the Helm release. Defaults to a composition of
&lsquo;[TargetNamespace-]Name&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>targetNamespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetNamespace to target when performing operations for the HelmRelease.
Defaults to the namespace of the HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>storageNamespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StorageNamespace used for the Helm storage.
Defaults to the namespace of the HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>dependsOn</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#NamespacedObjectReference">
[]github.com/fluxcd/pkg/apis/meta.NamespacedObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DependsOn may contain a meta.NamespacedObjectReference slice with
references to HelmRelease resources that must be ready before this HelmRelease
can be reconciled.</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation (like Jobs
for hooks) during the performance of a Helm action. Defaults to &lsquo;5m0s&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>maxHistory</code><br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxHistory is the number of revisions saved by Helm for this HelmRelease.
Use &lsquo;0&rsquo; for an unlimited number of revisions; defaults to &lsquo;5&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the Kubernetes service account to impersonate
when reconciling this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>persistentClient</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>PersistentClient tells the controller to use a persistent Kubernetes
client for this release. When enabled, the client will be reused for the
duration of the reconciliation, instead of being created and destroyed
for each (step of a) Helm action.</p>
<p>This can improve performance, but may cause issues with some Helm charts
that for example do create Custom Resource Definitions during installation
outside Helm&rsquo;s CRD lifecycle hooks, which are then not observed to be
available by e.g. post-install hooks.</p>
<p>If not set, it defaults to true.</p>
</td>
</tr>
<tr>
<td>
<code>driftDetection</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.DriftDetection">
DriftDetection
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DriftDetection holds the configuration for detecting and handling
differences between the manifest in the Helm storage and the resources
currently existing in the cluster.</p>
</td>
</tr>
<tr>
<td>
<code>install</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Install">
Install
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Install holds the configuration for Helm install actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>upgrade</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Upgrade">
Upgrade
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Upgrade holds the configuration for Helm upgrade actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>test</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Test">
Test
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Test holds the configuration for Helm test actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>rollback</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Rollback">
Rollback
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Rollback holds the configuration for Helm rollback actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>uninstall</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Uninstall">
Uninstall
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Uninstall holds the configuration for Helm uninstall actions for this HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>valuesFrom</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.ValuesReference">
[]ValuesReference
</a>
</em>
</td>
<td>
<p>ValuesFrom holds references to resources containing Helm values for this HelmRelease,
and information about how they should be merged.</p>
</td>
</tr>
<tr>
<td>
<code>values</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Values holds the values for this Helm release.</p>
</td>
</tr>
<tr>
<td>
<code>postRenderers</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.PostRenderer">
[]PostRenderer
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PostRenderers holds an array of Helm PostRenderers, which will be applied in order
of their definition.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.HelmReleaseStatus">HelmReleaseStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmRelease">HelmRelease</a>)
</p>
<p>HelmReleaseStatus defines the observed state of a HelmRelease.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>observedGeneration</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the last observed generation.</p>
</td>
</tr>
<tr>
<td>
<code>observedPostRenderersDigest</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedPostRenderersDigest is the digest for the post-renderers of
the last successful reconciliation attempt.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedGeneration</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedGeneration is the last generation the controller attempted
to reconcile.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions holds the conditions for the HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>helmChart</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>HelmChart is the namespaced name of the HelmChart resource created by
the controller for the HelmRelease.</p>
</td>
</tr>
<tr>
<td>
<code>storageNamespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StorageNamespace is the namespace of the Helm release storage for the
current release.</p>
</td>
</tr>
<tr>
<td>
<code>history</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Snapshots">
Snapshots
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>History holds the history of Helm releases performed for this HelmRelease
up to the last successfully completed release.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedReleaseAction</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.ReleaseAction">
ReleaseAction
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedReleaseAction is the last release action performed for this
HelmRelease. It is used to determine the active remediation strategy.</p>
</td>
</tr>
<tr>
<td>
<code>failures</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Failures is the reconciliation failure count against the latest desired
state. It is reset after a successful reconciliation.</p>
</td>
</tr>
<tr>
<td>
<code>installFailures</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>InstallFailures is the install failure count against the latest desired
state. It is reset after a successful reconciliation.</p>
</td>
</tr>
<tr>
<td>
<code>upgradeFailures</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>UpgradeFailures is the upgrade failure count against the latest desired
state. It is reset after a successful reconciliation.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedRevision is the Source revision of the last reconciliation
attempt. For OCIRepository  sources, the 12 first characters of the digest are
appended to the chart version e.g. &ldquo;1.2.3+1234567890ab&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedRevisionDigest</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedRevisionDigest is the digest of the last reconciliation attempt.
This is only set for OCIRepository sources.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedValuesChecksum</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedValuesChecksum is the SHA1 checksum for the values of the last
reconciliation attempt.
Deprecated: Use LastAttemptedConfigDigest instead.</p>
</td>
</tr>
<tr>
<td>
<code>lastReleaseRevision</code><br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastReleaseRevision is the revision of the last successful Helm release.
Deprecated: Use History instead.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedConfigDigest</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedConfigDigest is the digest for the config (better known as
&ldquo;values&rdquo;) of the last reconciliation attempt.</p>
</td>
</tr>
<tr>
<td>
<code>lastHandledForceAt</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastHandledForceAt holds the value of the most recent force request
value, so a change of the annotation value can be detected.</p>
</td>
</tr>
<tr>
<td>
<code>lastHandledResetAt</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastHandledResetAt holds the value of the most recent reset request
value, so a change of the annotation value can be detected.</p>
</td>
</tr>
<tr>
<td>
<code>ReconcileRequestStatus</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#ReconcileRequestStatus">
github.com/fluxcd/pkg/apis/meta.ReconcileRequestStatus
</a>
</em>
</td>
<td>
<p>
(Members of <code>ReconcileRequestStatus</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.IgnoreRule">IgnoreRule
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.DriftDetection">DriftDetection</a>)
</p>
<p>IgnoreRule defines a rule to selectively disregard specific changes during
the drift detection process.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>paths</code><br>
<em>
[]string
</em>
</td>
<td>
<p>Paths is a list of JSON Pointer (RFC 6901) paths to be excluded from
consideration in a Kubernetes object.</p>
</td>
</tr>
<tr>
<td>
<code>target</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/kustomize#Selector">
github.com/fluxcd/pkg/apis/kustomize.Selector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Target is a selector for specifying Kubernetes objects to which this
rule applies.
If Target is not set, the Paths will be ignored for all Kubernetes
objects within the manifest of the Helm release.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.Install">Install
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>Install holds the configuration for Helm install actions performed for this
HelmRelease.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation (like
Jobs for hooks) during the performance of a Helm install action. Defaults to
&lsquo;HelmReleaseSpec.Timeout&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>remediation</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.InstallRemediation">
InstallRemediation
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Remediation holds the remediation configuration for when the Helm install
action for the HelmRelease fails. The default is to not perform any action.</p>
</td>
</tr>
<tr>
<td>
<code>disableWait</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWait disables the waiting for resources to be ready after a Helm
install has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>disableWaitForJobs</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWaitForJobs disables waiting for jobs to complete after a Helm
install has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>disableHooks</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableHooks prevents hooks from running during the Helm install action.</p>
</td>
</tr>
<tr>
<td>
<code>disableOpenAPIValidation</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableOpenAPIValidation prevents the Helm install action from validating
rendered templates against the Kubernetes OpenAPI Schema.</p>
</td>
</tr>
<tr>
<td>
<code>replace</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Replace tells the Helm install action to re-use the &lsquo;ReleaseName&rsquo;, but only
if that name is a deleted release which remains in the history.</p>
</td>
</tr>
<tr>
<td>
<code>skipCRDs</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>SkipCRDs tells the Helm install action to not install any CRDs. By default,
CRDs are installed if not already present.</p>
<p>Deprecated use CRD policy (<code>crds</code>) attribute with value <code>Skip</code> instead.</p>
</td>
</tr>
<tr>
<td>
<code>crds</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.CRDsPolicy">
CRDsPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>CRDs upgrade CRDs from the Helm Chart&rsquo;s crds directory according
to the CRD upgrade policy provided here. Valid values are <code>Skip</code>,
<code>Create</code> or <code>CreateReplace</code>. Default is <code>Create</code> and if omitted
CRDs are installed but not updated.</p>
<p>Skip: do neither install nor replace (update) any CRDs.</p>
<p>Create: new CRDs are created, existing CRDs are neither updated nor deleted.</p>
<p>CreateReplace: new CRDs are created, existing CRDs are updated (replaced)
but not deleted.</p>
<p>By default, CRDs are applied (installed) during Helm install action.
With this option users can opt in to CRD replace existing CRDs on Helm
install actions, which is not (yet) natively supported by Helm.
<a href="https://helm.sh/docs/chart_best_practices/custom_resource_definitions">https://helm.sh/docs/chart_best_practices/custom_resource_definitions</a>.</p>
</td>
</tr>
<tr>
<td>
<code>createNamespace</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>CreateNamespace tells the Helm install action to create the
HelmReleaseSpec.TargetNamespace if it does not exist yet.
On uninstall, the namespace will not be garbage collected.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.InstallRemediation">InstallRemediation
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.Install">Install</a>)
</p>
<p>InstallRemediation holds the configuration for Helm install remediation.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>retries</code><br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Retries is the number of retries that should be attempted on failures before
bailing. Remediation, using an uninstall, is performed between each attempt.
Defaults to &lsquo;0&rsquo;, a negative integer equals to unlimited retries.</p>
</td>
</tr>
<tr>
<td>
<code>ignoreTestFailures</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>IgnoreTestFailures tells the controller to skip remediation when the Helm
tests are run after an install action but fail. Defaults to
&lsquo;Test.IgnoreFailures&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>remediateLastFailure</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RemediateLastFailure tells the controller to remediate the last failure, when
no retries remain. Defaults to &lsquo;false&rsquo;.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.Kustomize">Kustomize
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.PostRenderer">PostRenderer</a>)
</p>
<p>Kustomize Helm PostRenderer specification.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>patches</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/kustomize#Patch">
[]github.com/fluxcd/pkg/apis/kustomize.Patch
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Strategic merge and JSON patches, defined as inline YAML objects,
capable of targeting objects based on kind, label and annotation selectors.</p>
</td>
</tr>
<tr>
<td>
<code>images</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/kustomize#Image">
[]github.com/fluxcd/pkg/apis/kustomize.Image
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Images is a list of (image name, new name, new tag or digest)
for changing image names, tags or digests. This can also be achieved with a
patch, but this operator is simpler to specify.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.PostRenderer">PostRenderer
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>PostRenderer contains a Helm PostRenderer specification.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kustomize</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Kustomize">
Kustomize
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Kustomization to apply as PostRenderer.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.ReleaseAction">ReleaseAction
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseStatus">HelmReleaseStatus</a>)
</p>
<p>ReleaseAction is the action to perform a Helm release.</p>
<h3 id="helm.toolkit.fluxcd.io/v2.Remediation">Remediation
</h3>
<p>Remediation defines a consistent interface for InstallRemediation and
UpgradeRemediation.</p>
<h3 id="helm.toolkit.fluxcd.io/v2.RemediationStrategy">RemediationStrategy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.UpgradeRemediation">UpgradeRemediation</a>)
</p>
<p>RemediationStrategy returns the strategy to use to remediate a failed install
or upgrade.</p>
<h3 id="helm.toolkit.fluxcd.io/v2.Rollback">Rollback
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>Rollback holds the configuration for Helm rollback actions for this
HelmRelease.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation (like
Jobs for hooks) during the performance of a Helm rollback action. Defaults to
&lsquo;HelmReleaseSpec.Timeout&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>disableWait</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWait disables the waiting for resources to be ready after a Helm
rollback has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>disableWaitForJobs</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWaitForJobs disables waiting for jobs to complete after a Helm
rollback has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>disableHooks</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableHooks prevents hooks from running during the Helm rollback action.</p>
</td>
</tr>
<tr>
<td>
<code>recreate</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Recreate performs pod restarts for the resource if applicable.</p>
</td>
</tr>
<tr>
<td>
<code>force</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Force forces resource updates through a replacement strategy.</p>
</td>
</tr>
<tr>
<td>
<code>cleanupOnFail</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>CleanupOnFail allows deletion of new resources created during the Helm
rollback action when it fails.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.Snapshot">Snapshot
</h3>
<p>Snapshot captures a point-in-time copy of the status information for a Helm release,
as managed by the controller.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>APIVersion is the API version of the Snapshot.
Provisional: when the calculation method of the Digest field is changed,
this field will be used to distinguish between the old and new methods.</p>
</td>
</tr>
<tr>
<td>
<code>digest</code><br>
<em>
string
</em>
</td>
<td>
<p>Digest is the checksum of the release object in storage.
It has the format of <code>&lt;algo&gt;:&lt;checksum&gt;</code>.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the release.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br>
<em>
string
</em>
</td>
<td>
<p>Namespace is the namespace the release is deployed to.</p>
</td>
</tr>
<tr>
<td>
<code>version</code><br>
<em>
int
</em>
</td>
<td>
<p>Version is the version of the release object in storage.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br>
<em>
string
</em>
</td>
<td>
<p>Status is the current state of the release.</p>
</td>
</tr>
<tr>
<td>
<code>chartName</code><br>
<em>
string
</em>
</td>
<td>
<p>ChartName is the chart name of the release object in storage.</p>
</td>
</tr>
<tr>
<td>
<code>chartVersion</code><br>
<em>
string
</em>
</td>
<td>
<p>ChartVersion is the chart version of the release object in
storage.</p>
</td>
</tr>
<tr>
<td>
<code>appVersion</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>AppVersion is the chart app version of the release object in storage.</p>
</td>
</tr>
<tr>
<td>
<code>configDigest</code><br>
<em>
string
</em>
</td>
<td>
<p>ConfigDigest is the checksum of the config (better known as
&ldquo;values&rdquo;) of the release object in storage.
It has the format of <code>&lt;algo&gt;:&lt;checksum&gt;</code>.</p>
</td>
</tr>
<tr>
<td>
<code>firstDeployed</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<p>FirstDeployed is when the release was first deployed.</p>
</td>
</tr>
<tr>
<td>
<code>lastDeployed</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<p>LastDeployed is when the release was last deployed.</p>
</td>
</tr>
<tr>
<td>
<code>deleted</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Deleted is when the release was deleted.</p>
</td>
</tr>
<tr>
<td>
<code>testHooks</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.TestHookStatus">
TestHookStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>TestHooks is the list of test hooks for the release as observed to be
run by the controller.</p>
</td>
</tr>
<tr>
<td>
<code>ociDigest</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>OCIDigest is the digest of the OCI artifact associated with the release.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.Snapshots">Snapshots
(<code>[]*./api/v2.Snapshot</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseStatus">HelmReleaseStatus</a>)
</p>
<p>Snapshots is a list of Snapshot objects.</p>
<h3 id="helm.toolkit.fluxcd.io/v2.Test">Test
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>Test holds the configuration for Helm test actions for this HelmRelease.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enable</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enable enables Helm test actions for this HelmRelease after an Helm install
or upgrade action has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation during
the performance of a Helm test action. Defaults to &lsquo;HelmReleaseSpec.Timeout&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>ignoreFailures</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>IgnoreFailures tells the controller to skip remediation when the Helm tests
are run but fail. Can be overwritten for tests run after install or upgrade
actions in &lsquo;Install.IgnoreTestFailures&rsquo; and &lsquo;Upgrade.IgnoreTestFailures&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>filters</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.Filter">
Filter
</a>
</em>
</td>
<td>
<p>Filters is a list of tests to run or exclude from running.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.TestHookStatus">TestHookStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.Snapshot">Snapshot</a>)
</p>
<p>TestHookStatus holds the status information for a test hook as observed
to be run by the controller.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>lastStarted</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastStarted is the time the test hook was last started.</p>
</td>
</tr>
<tr>
<td>
<code>lastCompleted</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastCompleted is the time the test hook last completed.</p>
</td>
</tr>
<tr>
<td>
<code>phase</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Phase the test hook was observed to be in.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.Uninstall">Uninstall
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>Uninstall holds the configuration for Helm uninstall actions for this
HelmRelease.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation (like
Jobs for hooks) during the performance of a Helm uninstall action. Defaults
to &lsquo;HelmReleaseSpec.Timeout&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>disableHooks</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableHooks prevents hooks from running during the Helm rollback action.</p>
</td>
</tr>
<tr>
<td>
<code>keepHistory</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeepHistory tells Helm to remove all associated resources and mark the
release as deleted, but retain the release history.</p>
</td>
</tr>
<tr>
<td>
<code>disableWait</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWait disables waiting for all the resources to be deleted after
a Helm uninstall is performed.</p>
</td>
</tr>
<tr>
<td>
<code>deletionPropagation</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>DeletionPropagation specifies the deletion propagation policy when
a Helm uninstall is performed.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.Upgrade">Upgrade
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>Upgrade holds the configuration for Helm upgrade actions for this
HelmRelease.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout is the time to wait for any individual Kubernetes operation (like
Jobs for hooks) during the performance of a Helm upgrade action. Defaults to
&lsquo;HelmReleaseSpec.Timeout&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>remediation</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.UpgradeRemediation">
UpgradeRemediation
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Remediation holds the remediation configuration for when the Helm upgrade
action for the HelmRelease fails. The default is to not perform any action.</p>
</td>
</tr>
<tr>
<td>
<code>disableWait</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWait disables the waiting for resources to be ready after a Helm
upgrade has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>disableWaitForJobs</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableWaitForJobs disables waiting for jobs to complete after a Helm
upgrade has been performed.</p>
</td>
</tr>
<tr>
<td>
<code>disableHooks</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableHooks prevents hooks from running during the Helm upgrade action.</p>
</td>
</tr>
<tr>
<td>
<code>disableOpenAPIValidation</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableOpenAPIValidation prevents the Helm upgrade action from validating
rendered templates against the Kubernetes OpenAPI Schema.</p>
</td>
</tr>
<tr>
<td>
<code>force</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Force forces resource updates through a replacement strategy.</p>
</td>
</tr>
<tr>
<td>
<code>preserveValues</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>PreserveValues will make Helm reuse the last release&rsquo;s values and merge in
overrides from &lsquo;Values&rsquo;. Setting this flag makes the HelmRelease
non-declarative.</p>
</td>
</tr>
<tr>
<td>
<code>cleanupOnFail</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>CleanupOnFail allows deletion of new resources created during the Helm
upgrade action when it fails.</p>
</td>
</tr>
<tr>
<td>
<code>crds</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.CRDsPolicy">
CRDsPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>CRDs upgrade CRDs from the Helm Chart&rsquo;s crds directory according
to the CRD upgrade policy provided here. Valid values are <code>Skip</code>,
<code>Create</code> or <code>CreateReplace</code>. Default is <code>Skip</code> and if omitted
CRDs are neither installed nor upgraded.</p>
<p>Skip: do neither install nor replace (update) any CRDs.</p>
<p>Create: new CRDs are created, existing CRDs are neither updated nor deleted.</p>
<p>CreateReplace: new CRDs are created, existing CRDs are updated (replaced)
but not deleted.</p>
<p>By default, CRDs are not applied during Helm upgrade action. With this
option users can opt-in to CRD upgrade, which is not (yet) natively supported by Helm.
<a href="https://helm.sh/docs/chart_best_practices/custom_resource_definitions">https://helm.sh/docs/chart_best_practices/custom_resource_definitions</a>.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.UpgradeRemediation">UpgradeRemediation
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.Upgrade">Upgrade</a>)
</p>
<p>UpgradeRemediation holds the configuration for Helm upgrade remediation.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>retries</code><br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Retries is the number of retries that should be attempted on failures before
bailing. Remediation, using &lsquo;Strategy&rsquo;, is performed between each attempt.
Defaults to &lsquo;0&rsquo;, a negative integer equals to unlimited retries.</p>
</td>
</tr>
<tr>
<td>
<code>ignoreTestFailures</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>IgnoreTestFailures tells the controller to skip remediation when the Helm
tests are run after an upgrade action but fail.
Defaults to &lsquo;Test.IgnoreFailures&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>remediateLastFailure</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RemediateLastFailure tells the controller to remediate the last failure, when
no retries remain. Defaults to &lsquo;false&rsquo; unless &lsquo;Retries&rsquo; is greater than 0.</p>
</td>
</tr>
<tr>
<td>
<code>strategy</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2.RemediationStrategy">
RemediationStrategy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Strategy to use for failure remediation. Defaults to &lsquo;rollback&rsquo;.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2.ValuesReference">ValuesReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>ValuesReference contains a reference to a resource containing Helm values,
and optionally the key they can be found at.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the values referent, valid values are (&lsquo;Secret&rsquo;, &lsquo;ConfigMap&rsquo;).</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the values referent. Should reside in the same namespace as the
referring resource.</p>
</td>
</tr>
<tr>
<td>
<code>valuesKey</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ValuesKey is the data key where the values.yaml or a specific value can be
found at. Defaults to &lsquo;values.yaml&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>targetPath</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetPath is the YAML dot notation path the value should be merged at. When
set, the ValuesKey is expected to be a single flat value. Defaults to &lsquo;None&rsquo;,
which results in the values getting merged at the root.</p>
</td>
</tr>
<tr>
<td>
<code>optional</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional marks this ValuesReference as optional. When set, a not found error
for the values reference is ignored, but any ValuesKey, TargetPath or
transient error will still result in a reconciliation failure.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<div class="admonition note">
<p class="last">This page was automatically generated with <code>gen-crd-api-reference-docs</code></p>
</div>
