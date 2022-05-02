/*
Copyright 2022 The Flux authors

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
	"github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ChartTemplateChangePredicate detects changes to the v1beta2.HelmChart
// template embedded in v2beta1.HelmRelease objects.
type ChartTemplateChangePredicate struct {
	predicate.Funcs
}

func (ChartTemplateChangePredicate) Create(e event.CreateEvent) bool {
	if e.Object == nil {
		return false
	}
	obj, ok := e.Object.(*v2beta1.HelmRelease)
	if !ok {
		return false
	}
	if obj.Spec.Suspend {
		return false
	}
	return true
}

func (ChartTemplateChangePredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldObj, ok := e.ObjectOld.(*v2beta1.HelmRelease)
	if !ok {
		return false
	}
	newObj, ok := e.ObjectNew.(*v2beta1.HelmRelease)
	if !ok {
		return false
	}
	if newObj.Spec.Suspend {
		return false
	}

	return !cmp.Equal(oldObj.Spec.Chart, newObj.Spec.Chart)
}

func (ChartTemplateChangePredicate) Delete(e event.DeleteEvent) bool {
	if e.Object == nil {
		return false
	}
	obj, ok := e.Object.(*v2beta1.HelmRelease)
	if !ok {
		return false
	}
	if obj.Spec.Suspend {
		return false
	}
	return true
}
