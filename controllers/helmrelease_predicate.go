/*
Copyright 2020 The Flux CD contributors.

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

package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	sourcev1 "github.com/fluxcd/source-controller/api/v1alpha1"

	v2 "github.com/fluxcd/helm-controller/api/v2alpha1"
)

type HelmReleaseReconcileAtPredicate struct {
	predicate.Funcs
}

func (HelmReleaseReconcileAtPredicate) Update(e event.UpdateEvent) bool {
	// Ignore objects without metadata
	if e.MetaOld == nil || e.MetaNew == nil {
		return false
	}

	// Reconcile on spec changes
	if e.MetaNew.GetGeneration() != e.MetaOld.GetGeneration() {
		return true
	}

	// Handle ReconcileAt annotation
	if val, ok := e.MetaNew.GetAnnotations()[v2.ReconcileAtAnnotation]; ok {
		if valOld, okOld := e.MetaOld.GetAnnotations()[v2.ReconcileAtAnnotation]; okOld {
			return val != valOld
		}
		return true
	}
	return false
}

type HelmReleaseGarbageCollectPredicate struct {
	predicate.Funcs
	Client client.Client
	Config *rest.Config
	Log    logr.Logger
}

func (gc HelmReleaseGarbageCollectPredicate) Update(e event.UpdateEvent) bool {
	if oldHr, ok := e.ObjectOld.(*v2.HelmRelease); ok {
		if newHr, ok := e.ObjectNew.(*v2.HelmRelease); ok {
			if oldHr.Spec.Chart.GetNamespace(oldHr.Namespace) != newHr.Spec.Chart.GetNamespace(newHr.Namespace) {
				var hc sourcev1.HelmChart
				chartName := types.NamespacedName{
					Namespace: oldHr.Spec.Chart.GetNamespace(oldHr.Namespace),
					Name:      oldHr.GetHelmChartName(),
				}
				if err := gc.Client.Get(context.TODO(), chartName, &hc); err != nil {
					if err = gc.Client.Delete(context.TODO(), &hc); err != nil {
						gc.Log.Error(
							err,
							fmt.Sprintf("failed to garbage collect HelmChart '%s' for HelmRelease", chartName.String()),
							"helmrelease", fmt.Sprintf("%s/%s", oldHr.Namespace, oldHr.Name),
						)
					}
				}
			}
		}
	}
	return true
}

func (gc HelmReleaseGarbageCollectPredicate) Delete(e event.DeleteEvent) bool {
	if hr, ok := e.Object.(*v2.HelmRelease); ok {
		var hc sourcev1.HelmChart
		chartName := types.NamespacedName{
			Namespace: hr.Spec.Chart.GetNamespace(hr.Namespace),
			Name:      hr.GetHelmChartName(),
		}
		if err := gc.Client.Get(context.TODO(), chartName, &hc); err == nil {
			if err = gc.Client.Delete(context.TODO(), &hc); err != nil {
				gc.Log.Error(
					err,
					"failed to delete HelmChart for HelmRelease",
					"helmrelease",
					fmt.Sprintf("%s/%s", hr.Namespace, hr.Name),
				)
			}
		}
		if !hr.Spec.Suspend {
			cfg, err := newActionCfg(gc.Log, gc.Config, *hr)
			if err != nil {
				gc.Log.Error(
					err,
					"failed to initialize Helm action configuration for uninstall",
					"helmrelease",
					fmt.Sprintf("%s/%s", hr.Namespace, hr.Name),
				)
				return true
			}
			if _, err := cfg.Releases.Deployed(hr.Name); err != nil && errors.Is(err, driver.ErrNoDeployedReleases) {
				return true
			}
			if err := uninstall(cfg, *hr); err != nil {
				gc.Log.Error(
					err,
					"failed to uninstall Helm release",
					"helmrelease",
					fmt.Sprintf("%s/%s", hr.Namespace, hr.Name),
				)
				return true
			}
		}
	}
	return true
}
