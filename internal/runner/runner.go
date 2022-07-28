/*
Copyright 2021 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/postrender"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	runtimelogger "github.com/fluxcd/pkg/runtime/logger"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/helm-controller/internal/features"
	intpostrender "github.com/fluxcd/helm-controller/internal/postrender"
)

var accessor = meta.NewAccessor()

type ActionError struct {
	Err          error
	CapturedLogs string
}

func (e ActionError) Error() string {
	return e.Err.Error()
}

func (e ActionError) Unwrap() error {
	return e.Err
}

// Runner represents a Helm action runner capable of performing Helm
// operations for a v2beta1.HelmRelease.
type Runner struct {
	mu        sync.Mutex
	config    *action.Configuration
	logBuffer *LogBuffer
}

// NewRunner constructs a new Runner configured to run Helm actions with the
// given genericclioptions.RESTClientGetter, and the release and storage
// namespace configured to the provided values.
func NewRunner(getter genericclioptions.RESTClientGetter, storageNamespace string, logger logr.Logger) (*Runner, error) {
	runner := &Runner{
		logBuffer: NewLogBuffer(NewDebugLog(logger.V(runtimelogger.DebugLevel)), defaultBufferSize),
	}

	// Default to the trace level logger for the Helm action configuration,
	// to ensure storage logs are captured.
	cfg := new(action.Configuration)
	if err := cfg.Init(getter, storageNamespace, "secret", NewDebugLog(logger.V(runtimelogger.TraceLevel))); err != nil {
		return nil, err
	}

	// Override the logger used by the Helm actions and Kube client with the log buffer,
	// which provides useful information in the event of an error.
	cfg.Log = runner.logBuffer.Log
	if kc, ok := cfg.KubeClient.(*kube.Client); ok {
		kc.Log = runner.logBuffer.Log
	}
	runner.config = cfg

	return runner, nil
}

// Create post renderer instances from HelmRelease and combine them into
// a single combined post renderer.
func postRenderers(hr v2.HelmRelease) (postrender.PostRenderer, error) {
	renderers := make([]postrender.PostRenderer, 0)
	for _, r := range hr.Spec.PostRenderers {
		if r.Kustomize != nil {
			renderers = append(renderers, &intpostrender.Kustomize{
				Patches:               r.Kustomize.Patches,
				PatchesStrategicMerge: r.Kustomize.PatchesStrategicMerge,
				PatchesJSON6902:       r.Kustomize.PatchesJSON6902,
				Images:                r.Kustomize.Images,
			})
		}
	}
	renderers = append(renderers, intpostrender.NewOriginLabels(v2.GroupVersion.Group, hr.Namespace, hr.Name))
	if len(renderers) == 0 {
		return nil, nil
	}
	return intpostrender.NewCombined(renderers...), nil
}

// Install runs a Helm install action for the given v2beta1.HelmRelease.
func (r *Runner) Install(ctx context.Context, hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (*release.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	install := action.NewInstall(r.config)
	install.ReleaseName = hr.GetReleaseName()
	install.Namespace = hr.GetReleaseNamespace()
	install.Timeout = hr.Spec.GetInstall().GetTimeout(hr.GetTimeout()).Duration
	install.Wait = !hr.Spec.GetInstall().DisableWait
	install.WaitForJobs = !hr.Spec.GetInstall().DisableWaitForJobs
	install.DisableHooks = hr.Spec.GetInstall().DisableHooks
	install.DisableOpenAPIValidation = hr.Spec.GetInstall().DisableOpenAPIValidation
	install.Replace = hr.Spec.GetInstall().Replace
	install.SkipCRDs = true
	install.Devel = true

	if hr.Spec.TargetNamespace != "" {
		install.CreateNamespace = hr.Spec.GetInstall().CreateNamespace
	}

	// If user opted-in to allow DNS lookups, enable it.
	if allowDNS, _ := features.Enabled(features.AllowDNSLookups); allowDNS {
		install.EnableDNS = allowDNS
	}

	renderer, err := postRenderers(hr)
	if err != nil {
		return nil, wrapActionErr(r.logBuffer, err)
	}
	install.PostRenderer = renderer

	// If user opted-in to install (or replace) CRDs, install them first.
	var legacyCRDsPolicy = v2.Create
	if hr.Spec.GetInstall().SkipCRDs {
		legacyCRDsPolicy = v2.Skip
	}
	cRDsPolicy, err := r.validateCRDsPolicy(hr.Spec.GetInstall().CRDs, legacyCRDsPolicy)
	if err != nil {
		return nil, wrapActionErr(r.logBuffer, err)
	}
	if cRDsPolicy != v2.Skip && len(chart.CRDObjects()) > 0 {
		if err := r.applyCRDs(cRDsPolicy, chart, setOriginVisitor(hr.Namespace, hr.Name)); err != nil {
			return nil, wrapActionErr(r.logBuffer, err)
		}
	}

	rel, err := install.RunWithContext(ctx, chart, values.AsMap())
	return rel, wrapActionErr(r.logBuffer, err)
}

// Upgrade runs an Helm upgrade action for the given v2beta1.HelmRelease.
func (r *Runner) Upgrade(ctx context.Context, hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (*release.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	upgrade := action.NewUpgrade(r.config)
	upgrade.Namespace = hr.GetReleaseNamespace()
	upgrade.ResetValues = !hr.Spec.GetUpgrade().PreserveValues
	upgrade.ReuseValues = hr.Spec.GetUpgrade().PreserveValues
	upgrade.MaxHistory = hr.GetMaxHistory()
	upgrade.Timeout = hr.Spec.GetUpgrade().GetTimeout(hr.GetTimeout()).Duration
	upgrade.Wait = !hr.Spec.GetUpgrade().DisableWait
	upgrade.WaitForJobs = !hr.Spec.GetUpgrade().DisableWaitForJobs
	upgrade.DisableHooks = hr.Spec.GetUpgrade().DisableHooks
	upgrade.DisableOpenAPIValidation = hr.Spec.GetUpgrade().DisableOpenAPIValidation
	upgrade.Force = hr.Spec.GetUpgrade().Force
	upgrade.CleanupOnFail = hr.Spec.GetUpgrade().CleanupOnFail
	upgrade.Devel = true

	// If user opted-in to allow DNS lookups, enable it.
	if allowDNS, _ := features.Enabled(features.AllowDNSLookups); allowDNS {
		upgrade.EnableDNS = allowDNS
	}

	renderer, err := postRenderers(hr)
	if err != nil {
		return nil, wrapActionErr(r.logBuffer, err)
	}
	upgrade.PostRenderer = renderer

	// If user opted-in to upgrade CRDs, upgrade them first.
	cRDsPolicy, err := r.validateCRDsPolicy(hr.Spec.GetUpgrade().CRDs, v2.Skip)
	if err != nil {
		return nil, wrapActionErr(r.logBuffer, err)
	}
	if cRDsPolicy != v2.Skip && len(chart.CRDObjects()) > 0 {
		if err := r.applyCRDs(cRDsPolicy, chart, setOriginVisitor(hr.Namespace, hr.Name)); err != nil {
			return nil, wrapActionErr(r.logBuffer, err)
		}
	}

	rel, err := upgrade.RunWithContext(ctx, hr.GetReleaseName(), chart, values.AsMap())
	return rel, wrapActionErr(r.logBuffer, err)
}

func (r *Runner) validateCRDsPolicy(policy v2.CRDsPolicy, defaultValue v2.CRDsPolicy) (v2.CRDsPolicy, error) {
	switch policy {
	case "":
		return defaultValue, nil
	case v2.Skip:
		break
	case v2.Create:
		break
	case v2.CreateReplace:
		break
	default:
		return policy, fmt.Errorf("invalid CRD policy '%s' defined in field CRDsPolicy, valid values are '%s', '%s' or '%s'",
			policy, v2.Skip, v2.Create, v2.CreateReplace,
		)
	}
	return policy, nil
}

type rootScoped struct{}

func (*rootScoped) Name() meta.RESTScopeName {
	return meta.RESTScopeNameRoot
}

// This has been adapted from https://github.com/helm/helm/blob/v3.5.4/pkg/action/install.go#L127
func (r *Runner) applyCRDs(policy v2.CRDsPolicy, chart *chart.Chart, visitorFunc ...resource.VisitorFunc) error {
	r.config.Log("apply CRDs with policy %s", policy)

	// Collect all CRDs from all files in `crds` directory.
	allCrds := make(kube.ResourceList, 0)
	for _, obj := range chart.CRDObjects() {
		// Read in the resources
		res, err := r.config.KubeClient.Build(bytes.NewBuffer(obj.File.Data), false)
		if err != nil {
			r.config.Log("failed to parse CRDs from %s: %s", obj.Name, err)
			return fmt.Errorf("failed to parse CRDs from %s: %w", obj.Name, err)
		}
		allCrds = append(allCrds, res...)
	}

	// Visit CRDs with any provided visitor functions.
	for _, visitor := range visitorFunc {
		if err := allCrds.Visit(visitor); err != nil {
			return err
		}
	}

	var totalItems []*resource.Info
	switch policy {
	case v2.Skip:
		break
	case v2.Create:
		for i := range allCrds {
			if rr, err := r.config.KubeClient.Create(allCrds[i : i+1]); err != nil {
				crdName := allCrds[i].Name
				// If the error is CRD already exists, continue.
				if apierrors.IsAlreadyExists(err) {
					r.config.Log("CRD %s is already present. Skipping.", crdName)
					if rr != nil && rr.Created != nil {
						totalItems = append(totalItems, rr.Created...)
					}
					continue
				}
				r.config.Log("failed to create CRD %s: %s", crdName, err)
				return fmt.Errorf("failed to create CRD %s: %w", crdName, err)
			} else {
				if rr != nil && rr.Created != nil {
					totalItems = append(totalItems, rr.Created...)
				}
			}
		}
	case v2.CreateReplace:
		config, err := r.config.RESTClientGetter.ToRESTConfig()
		if err != nil {
			r.config.Log("Error while creating Kubernetes client config: %s", err)
			return err
		}
		clientset, err := apiextension.NewForConfig(config)
		if err != nil {
			r.config.Log("Error while creating Kubernetes clientset for apiextension: %s", err)
			return err
		}
		client := clientset.ApiextensionsV1().CustomResourceDefinitions()
		original := make(kube.ResourceList, 0)
		// Note, we build the originals from the current set of CRDs
		// and therefore this upgrade will never delete CRDs that existed in the former release
		// but no longer exist in the current release.
		for _, res := range allCrds {
			if o, err := client.Get(context.TODO(), res.Name, metav1.GetOptions{}); err == nil && o != nil {
				o.GetResourceVersion()
				original = append(original, &resource.Info{
					Client: clientset.ApiextensionsV1().RESTClient(),
					Mapping: &meta.RESTMapping{
						Resource: schema.GroupVersionResource{
							Group:    "apiextensions.k8s.io",
							Version:  res.Mapping.GroupVersionKind.Version,
							Resource: "customresourcedefinition",
						},
						GroupVersionKind: schema.GroupVersionKind{
							Kind:    "CustomResourceDefinition",
							Group:   "apiextensions.k8s.io",
							Version: res.Mapping.GroupVersionKind.Version,
						},
						Scope: &rootScoped{},
					},
					Namespace:       o.ObjectMeta.Namespace,
					Name:            o.ObjectMeta.Name,
					Object:          o,
					ResourceVersion: o.ObjectMeta.ResourceVersion,
				})
			} else if !apierrors.IsNotFound(err) {
				r.config.Log("failed to get CRD %s: %s", res.Name, err)
				return err
			}
		}
		// Send them to Kube
		if rr, err := r.config.KubeClient.Update(original, allCrds, true); err != nil {
			r.config.Log("failed to apply CRD %s", err)
			return fmt.Errorf("failed to apply CRD: %w", err)
		} else {
			if rr != nil {
				if rr.Created != nil {
					totalItems = append(totalItems, rr.Created...)
				}
				if rr.Updated != nil {
					totalItems = append(totalItems, rr.Updated...)
				}
				if rr.Deleted != nil {
					totalItems = append(totalItems, rr.Deleted...)
				}
			}
		}
	}

	if len(totalItems) > 0 {
		// Give time for the CRD to be recognized.
		if err := r.config.KubeClient.Wait(totalItems, 60*time.Second); err != nil {
			r.config.Log("Error waiting for items: %s", err)
			return err
		}

		// Clear the RESTMapper cache, since it will not have the new CRDs.
		// Further invalidation of the client is done at a later stage by Helm
		// when it gathers the server capabilities.
		if m, err := r.config.RESTClientGetter.ToRESTMapper(); err == nil {
			if rm, ok := m.(meta.ResettableRESTMapper); ok {
				r.config.Log("Clearing REST mapper cache")
				rm.Reset()
			}
		}
	}
	return nil
}

// Test runs an Helm test action for the given v2beta1.HelmRelease.
func (r *Runner) Test(hr v2.HelmRelease) (*release.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	test := action.NewReleaseTesting(r.config)
	test.Namespace = hr.GetReleaseNamespace()
	test.Timeout = hr.Spec.GetTest().GetTimeout(hr.GetTimeout()).Duration

	filters := make(map[string][]string)

	for _, f := range hr.Spec.GetTest().GetFilters() {
		name := "name"

		if f.Exclude {
			name = fmt.Sprintf("!%s", name)
		}

		filters[name] = append(filters[name], f.Name)
	}

	test.Filters = filters

	rel, err := test.Run(hr.GetReleaseName())
	return rel, wrapActionErr(r.logBuffer, err)
}

// Rollback runs an Helm rollback action for the given v2beta1.HelmRelease.
func (r *Runner) Rollback(hr v2.HelmRelease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	rollback := action.NewRollback(r.config)
	rollback.Timeout = hr.Spec.GetRollback().GetTimeout(hr.GetTimeout()).Duration
	rollback.Wait = !hr.Spec.GetRollback().DisableWait
	rollback.WaitForJobs = !hr.Spec.GetRollback().DisableWaitForJobs
	rollback.DisableHooks = hr.Spec.GetRollback().DisableHooks
	rollback.Force = hr.Spec.GetRollback().Force
	rollback.Recreate = hr.Spec.GetRollback().Recreate
	rollback.CleanupOnFail = hr.Spec.GetRollback().CleanupOnFail

	err := rollback.Run(hr.GetReleaseName())
	return wrapActionErr(r.logBuffer, err)
}

// Uninstall runs an Helm uninstall action for the given v2beta1.HelmRelease.
func (r *Runner) Uninstall(hr v2.HelmRelease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	uninstall := action.NewUninstall(r.config)
	uninstall.Timeout = hr.Spec.GetUninstall().GetTimeout(hr.GetTimeout()).Duration
	uninstall.DisableHooks = hr.Spec.GetUninstall().DisableHooks
	uninstall.KeepHistory = hr.Spec.GetUninstall().KeepHistory
	uninstall.DeletionPropagation = hr.Spec.GetUninstall().GetDeletionPropagation()
	uninstall.Wait = !hr.Spec.GetUninstall().DisableWait

	_, err := uninstall.Run(hr.GetReleaseName())
	return wrapActionErr(r.logBuffer, err)
}

// ObserveLastRelease observes the last revision, if there is one,
// for the actual Helm release associated with the given v2beta1.HelmRelease.
func (r *Runner) ObserveLastRelease(hr v2.HelmRelease) (*release.Release, error) {
	rel, err := r.config.Releases.Last(hr.GetReleaseName())
	if err != nil && errors.Is(err, driver.ErrReleaseNotFound) {
		err = nil
	}
	return rel, err
}

func wrapActionErr(log *LogBuffer, err error) error {
	if err == nil {
		return err
	}
	err = &ActionError{
		Err:          err,
		CapturedLogs: log.String(),
	}
	return err
}

func setOriginVisitor(namespace, name string) resource.VisitorFunc {
	return func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		if err = mergeLabels(info.Object, originLabels(name, namespace)); err != nil {
			return fmt.Errorf(
				"%s origin labels could not be updated: %s",
				resourceString(info), err,
			)
		}
		return nil
	}
}

func mergeLabels(obj runtime.Object, labels map[string]string) error {
	current, err := accessor.Labels(obj)
	if err != nil {
		return err
	}
	return accessor.SetLabels(obj, mergeStrStrMaps(current, labels))
}

func originLabels(name, namespace string) map[string]string {
	return map[string]string{
		fmt.Sprintf("%s/name", v2.GroupVersion.Group):      name,
		fmt.Sprintf("%s/namespace", v2.GroupVersion.Group): namespace,
	}
}

func resourceString(info *resource.Info) string {
	_, k := info.Mapping.GroupVersionKind.ToAPIVersionAndKind()
	return fmt.Sprintf(
		"%s %q in namespace %q",
		k, info.Name, info.Namespace,
	)
}

func mergeStrStrMaps(current, desired map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range current {
		result[k] = v
	}
	for k, desiredVal := range desired {
		result[k] = desiredVal
	}
	return result
}
