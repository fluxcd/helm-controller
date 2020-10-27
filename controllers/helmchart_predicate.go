/*
Copyright 2020 The Flux authors

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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
)

type HelmChartRevisionChangePredicate struct {
	predicate.Funcs
}

func (HelmChartRevisionChangePredicate) Update(e event.UpdateEvent) bool {
	if e.MetaOld == nil || e.MetaNew == nil {
		return false
	}

	oldChart, ok := e.ObjectOld.(*sourcev1.HelmChart)
	if !ok {
		return false
	}

	newChart, ok := e.ObjectNew.(*sourcev1.HelmChart)
	if !ok {
		return false
	}

	if oldChart.GetArtifact() == nil && newChart.GetArtifact() != nil {
		return true
	}

	if oldChart.GetArtifact() != nil && newChart.GetArtifact() != nil &&
		oldChart.GetArtifact().Revision != newChart.GetArtifact().Revision {
		return true
	}

	return false
}

func (HelmChartRevisionChangePredicate) Create(e event.CreateEvent) bool {
	return false
}

func (HelmChartRevisionChangePredicate) Delete(e event.DeleteEvent) bool {
	return false
}
