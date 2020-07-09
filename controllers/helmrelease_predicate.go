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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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
	Config *rest.Config
	Log    logr.Logger
}

func (gc HelmReleaseGarbageCollectPredicate) Delete(e event.DeleteEvent) bool {
	if hr, ok := e.Object.(*v2.HelmRelease); ok {
		cfg, err := newActionCfg(gc.Log, gc.Config, *hr)
		if err != nil {
			gc.Log.Error(err, "failed to initialize Helm action configuration for uninstall", "helmrelease", fmt.Sprintf("%s/%s", hr.Namespace, hr.Name))
			return false
		}
		if _, err := cfg.Releases.Deployed(hr.Name); err != nil && errors.Is(err, driver.ErrNoDeployedReleases) {
			return true
		}
		if err := uninstall(cfg, *hr); err != nil {
			gc.Log.Error(err, "failed to uninstall Helm release", "helmrelease", fmt.Sprintf("%s/%s", hr.Namespace, hr.Name))
			return false
		}
	}
	return true
}
