<h1>HelmRelease API reference</h1>
<p>Packages:</p>
<ul class="simple">
<li>
<a href="#helm.toolkit.fluxcd.io%2fv2alpha1">helm.toolkit.fluxcd.io/v2alpha1</a>
</li>
</ul>
<h2 id="helm.toolkit.fluxcd.io/v2alpha1">helm.toolkit.fluxcd.io/v2alpha1</h2>
<p>Package v2alpha1 contains API Schema definitions for the helm v2alpha1 API group</p>
Resource Types:
<ul class="simple"><li>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmRelease">HelmRelease</a>
</li></ul>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.HelmRelease">HelmRelease
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
<code>helm.toolkit.fluxcd.io/v2alpha1</code>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#objectmeta-v1-meta">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplate">
HelmChartTemplate
</a>
</em>
</td>
<td>
<p>Chart defines the template of the v1alpha1.HelmChart that should be created
for this HelmRelease.</p>
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
<code>dependsOn</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/runtime/dependency#CrossNamespaceDependencyReference">
[]Runtime dependency.CrossNamespaceDependencyReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DependsOn may contain a dependency.CrossNamespaceDependencyReference slice with
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
Use &lsquo;0&rsquo; for an unlimited number of revisions; defaults to &lsquo;10&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>install</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Install">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Upgrade">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Test">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Rollback">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Uninstall">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.ValuesReference">
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
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseStatus">
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
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.CrossNamespaceObjectReference">CrossNamespaceObjectReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplateSpec">HelmChartTemplateSpec</a>)
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
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.DeploymentAction">DeploymentAction
</h3>
<p>DeploymentAction defines a consistent interface for Install and Upgrade.</p>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplate">HelmChartTemplate
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
</p>
<p>HelmChartTemplate defines the template from which the controller will
generate a v1alpha1.HelmChart object in the same namespace as the referenced
v1alpha1.Source.</p>
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
<code>spec</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplateSpec">
HelmChartTemplateSpec
</a>
</em>
</td>
<td>
<p>Spec holds the template for the v1alpha1.HelmChartSpec for this HelmRelease.</p>
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
<p>Version semver expression, ignored for charts from v1alpha1.GitRepository and
v1alpha1.Bucket sources. Defaults to latest when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.CrossNamespaceObjectReference">
CrossNamespaceObjectReference
</a>
</em>
</td>
<td>
<p>The name and namespace of the v1alpha1.Source the chart is available at.</p>
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
<p>Interval at which to check the v1alpha1.Source for updates. Defaults to
&lsquo;HelmReleaseSpec.Interval&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>valuesFile</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Alternative values file to use as the default chart values, expected to be a
relative path in the SourceRef. Ignored when omitted.</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplateSpec">HelmChartTemplateSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplate">HelmChartTemplate</a>)
</p>
<p>HelmChartTemplateSpec defines the template from which the controller will
generate a v1alpha1.HelmChartSpec object.</p>
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
<p>Version semver expression, ignored for charts from v1alpha1.GitRepository and
v1alpha1.Bucket sources. Defaults to latest when omitted.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.CrossNamespaceObjectReference">
CrossNamespaceObjectReference
</a>
</em>
</td>
<td>
<p>The name and namespace of the v1alpha1.Source the chart is available at.</p>
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
<p>Interval at which to check the v1alpha1.Source for updates. Defaults to
&lsquo;HelmReleaseSpec.Interval&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>valuesFile</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Alternative values file to use as the default chart values, expected to be a
relative path in the SourceRef. Ignored when omitted.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmRelease">HelmRelease</a>)
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmChartTemplate">
HelmChartTemplate
</a>
</em>
</td>
<td>
<p>Chart defines the template of the v1alpha1.HelmChart that should be created
for this HelmRelease.</p>
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
<code>dependsOn</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/runtime/dependency#CrossNamespaceDependencyReference">
[]Runtime dependency.CrossNamespaceDependencyReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DependsOn may contain a dependency.CrossNamespaceDependencyReference slice with
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
Use &lsquo;0&rsquo; for an unlimited number of revisions; defaults to &lsquo;10&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>install</code><br>
<em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Install">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Upgrade">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Test">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Rollback">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Uninstall">
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.ValuesReference">
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
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseStatus">HelmReleaseStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmRelease">HelmRelease</a>)
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
<code>lastHandledReconcileAt</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastHandledReconcileAt is the last manual reconciliation request (by
annotating the HelmRelease) handled by the reconciler.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#Condition">
[]github.com/fluxcd/pkg/apis/meta.Condition
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
<code>lastAppliedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAppliedRevision is the revision of the last successfully applied source.</p>
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
<p>LastAttemptedRevision is the revision of the last reconciliation attempt.</p>
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
<p>LastAttemptedValuesChecksum is the SHA1 checksum of the values of the last
reconciliation attempt.</p>
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
<p>LastReleaseRevision is the revision of the last successful Helm release.</p>
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
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.Install">Install
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.InstallRemediation">
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
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.InstallRemediation">InstallRemediation
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Install">Install</a>)
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
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.Remediation">Remediation
</h3>
<p>Remediation defines a consistent interface for InstallRemediation and
UpgradeRemediation.</p>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.RemediationStrategy">RemediationStrategy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.UpgradeRemediation">UpgradeRemediation</a>)
</p>
<p>RemediationStrategy returns the strategy to use to remediate a failed install
or upgrade.</p>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.Rollback">Rollback
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
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
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.Test">Test
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
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
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.Uninstall">Uninstall
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
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
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.Upgrade">Upgrade
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.UpgradeRemediation">
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
</tbody>
</table>
</div>
</div>
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.UpgradeRemediation">UpgradeRemediation
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.Upgrade">Upgrade</a>)
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
<a href="#helm.toolkit.fluxcd.io/v2alpha1.RemediationStrategy">
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
<h3 id="helm.toolkit.fluxcd.io/v2alpha1.ValuesReference">ValuesReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#helm.toolkit.fluxcd.io/v2alpha1.HelmReleaseSpec">HelmReleaseSpec</a>)
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
