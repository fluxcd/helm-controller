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

package predicates

import (
	"github.com/fluxcd/pkg/runtime/conditions"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

// SourceRevisionChangePredicate detects revision changes to the v1.Artifact of
// a v1.Source object.
type SourceRevisionChangePredicate struct {
	predicate.Funcs
}

func (SourceRevisionChangePredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldSource, ok := e.ObjectOld.(sourcev1.Source)
	if !ok {
		return false
	}

	newSource, ok := e.ObjectNew.(sourcev1.Source)
	if !ok {
		return false
	}

	if oldSource.GetArtifact() == nil && newSource.GetArtifact() != nil {
		return true
	}

	if oldSource.GetArtifact() != nil && newSource.GetArtifact() != nil &&
		!oldSource.GetArtifact().HasRevision(newSource.GetArtifact().Revision) {
		return true
	}

	oldConditions, ok := e.ObjectOld.(conditions.Getter)
	if !ok {
		return false
	}

	newConditions, ok := e.ObjectNew.(conditions.Getter)
	if !ok {
		return false
	}

	if !conditions.IsReady(oldConditions) && conditions.IsReady(newConditions) {
		return true
	}

	return false
}

func (SourceRevisionChangePredicate) Create(e event.CreateEvent) bool {
	return false
}

func (SourceRevisionChangePredicate) Delete(e event.DeleteEvent) bool {
	return false
}
