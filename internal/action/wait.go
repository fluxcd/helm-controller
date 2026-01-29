/*
Copyright 2026 The Flux authors

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

package action

import (
	"context"
	"time"

	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/engine"
	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/statusreaders"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/fluxcd/cli-utils/pkg/object"
	helmkube "helm.sh/helm/v4/pkg/kube"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fluxcd/pkg/runtime/controller"
	runtimestatusreaders "github.com/fluxcd/pkg/runtime/statusreaders"
	"github.com/fluxcd/pkg/ssa"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// actionThatWaits is implemented by HelmRelease action specs that
// support wait strategies.
type actionThatWaits interface {
	GetDisableWait() bool
}

// getWaitStrategy returns the wait strategy for the given action spec.
func getWaitStrategy(strategy v2.WaitStrategyName, spec actionThatWaits) helmkube.WaitStrategy {
	switch {
	case spec.GetDisableWait():
		return helmkube.HookOnlyStrategy
	case strategy == v2.WaitStrategyPoller: // We hijack the Watcher name to mean Poller.
		return helmkube.StatusWatcherStrategy
	case strategy != "":
		return helmkube.WaitStrategy(strategy)
	case UseHelm3Defaults:
		return helmkube.LegacyStrategy
	default:
		return helmkube.StatusWatcherStrategy
	}
}

// NewStatusReaderFunc is a function type that returns a kstatus StatusReader.
type NewStatusReaderFunc = func(apimeta.RESTMapper) engine.StatusReader

// waiter waits for resources using kstatus polling.
type waiter struct {
	c                  *helmkube.Client
	strategy           helmkube.WaitStrategy
	newResourceManager func(sr ...NewStatusReaderFunc) *ssa.ResourceManager
	waitContext        context.Context
}

// waitCtx returns the wait context, defaulting to context.Background()
// when no context was configured via WithWaitContext.
func (w *waiter) waitCtx() context.Context {
	if w.waitContext != nil {
		return w.waitContext
	}
	return context.Background()
}

// WatchUntilReady implements kube.Waiter.
// Helm uses this method for hooks.
func (w *waiter) WatchUntilReady(resources helmkube.ResourceList, timeout time.Duration) error {
	const failFast = false // We never want to fail fast for hooks as they could be database migrations etc.
	return w.wait(w.waitCtx(), resources, timeout, failFast,
		// Helm docs say that Jobs and Pods are waited on during hooks,
		// but everything else is always considered ready. Except for
		// custom status readers built into w.newResourceManager, which
		// take precedence (the Helm `watcher` strategy does the same).
		runtimestatusreaders.NewCustomJobStatusReader,
		NewCustomPodStatusReader,
		alwaysReady)
}

// alwaysReady is copied from Helm for waiting on hooks.
func alwaysReady(restMapper apimeta.RESTMapper) engine.StatusReader {
	return statusreaders.NewGenericStatusReader(restMapper, func(_ *unstructured.Unstructured) (*status.Result, error) {
		return &status.Result{
			Status:  status.CurrentStatus,
			Message: "Resource is current",
		}, nil
	})
}

// Wait implements kube.Waiter. Not used for hooks.
func (w *waiter) Wait(resources helmkube.ResourceList, timeout time.Duration) error {
	if w.strategy == helmkube.HookOnlyStrategy {
		return nil
	}
	ctx := controller.GetInterruptContext(w.waitCtx())
	const failFast = true
	return w.wait(ctx, resources, timeout, failFast)
}

// WaitWithJobs implements kube.Waiter. Not used for hooks.
func (w *waiter) WaitWithJobs(resources helmkube.ResourceList, timeout time.Duration) error {
	if w.strategy == helmkube.HookOnlyStrategy {
		return nil
	}
	ctx := controller.GetInterruptContext(w.waitCtx())
	const failFast = true
	return w.wait(ctx, resources, timeout, failFast,
		runtimestatusreaders.NewCustomJobStatusReader)
}

// WaitForDelete implements kube.Waiter. Not used for hooks, used only for deletion.
func (w *waiter) WaitForDelete(resources helmkube.ResourceList, timeout time.Duration) error {
	if w.strategy == helmkube.HookOnlyStrategy {
		return nil
	}

	// WaitForTermination expects a list of Unstructured.
	objs := make([]*unstructured.Unstructured, 0, len(resources))
	for _, r := range resources {
		gvk := r.Object.GetObjectKind().GroupVersionKind()
		var o unstructured.Unstructured
		o.SetGroupVersionKind(gvk)
		o.SetNamespace(r.Namespace)
		o.SetName(r.Name)
		objs = append(objs, &o)
	}

	return w.newResourceManager().WaitForTermination(objs, ssa.WaitOptions{
		Interval: 2 * time.Second, // Copied from kustomize-controller.
		Timeout:  timeout,
	})
}

// wait is a helper method to wait for resources using the given status readers.
func (w *waiter) wait(ctx context.Context, resources helmkube.ResourceList,
	timeout time.Duration, failFast bool, sr ...NewStatusReaderFunc) error {

	// WaitForSetWithContext expects a list of ObjMetadata.
	objs := []object.ObjMetadata{}
	for _, res := range resources {
		// Skip paused apps/v1/Deployment (copied from Helm).
		gvk := res.Object.GetObjectKind().GroupVersionKind()
		if gvk == deploymentGVK {
			uns, err := runtime.DefaultUnstructuredConverter.ToUnstructured(res.Object)
			if err != nil {
				return err
			}
			paused, ok, _ := unstructured.NestedBool(uns, "spec", "paused")
			if ok && paused {
				continue
			}
		}

		// Convert to ObjMetadata.
		obj, err := object.RuntimeToObjMeta(res.Object)
		if err != nil {
			return err
		}
		objs = append(objs, obj)
	}

	return w.newResourceManager(sr...).WaitForSetWithContext(ctx, objs, ssa.WaitOptions{
		Interval: 5 * time.Second, // Copied from kustomize-controller.
		Timeout:  timeout,
		// The kustomize-controller has an opt-in feature gate that disables
		// fail fast here: DisableFailFastBehavior.
		FailFast: failFast,
	})
}

var deploymentGVK = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
