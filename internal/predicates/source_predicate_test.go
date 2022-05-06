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

package predicates

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

func TestSourceRevisionChangePredicate_Update(t *testing.T) {
	sourceA := &sourceMock{revision: "revision-a"}
	sourceB := &sourceMock{revision: "revision-b"}
	emptySource := &sourceMock{}
	notASource := &unstructured.Unstructured{}

	tests := []struct {
		name string
		old  client.Object
		new  client.Object
		want bool
	}{
		{name: "same artifact revision", old: sourceA, new: sourceA, want: false},
		{name: "diff artifact revision", old: sourceA, new: sourceB, want: true},
		{name: "new with artifact", old: emptySource, new: sourceA, want: true},
		{name: "old with artifact", old: sourceA, new: emptySource, want: false},
		{name: "old not a source", old: notASource, new: sourceA, want: false},
		{name: "new not a source", old: sourceA, new: notASource, want: false},
		{name: "old nil", old: nil, new: sourceA, want: false},
		{name: "new nil", old: sourceA, new: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			so := SourceRevisionChangePredicate{}
			e := event.UpdateEvent{
				ObjectOld: tt.old,
				ObjectNew: tt.new,
			}
			g.Expect(so.Update(e)).To(gomega.Equal(tt.want))
		})
	}
}

type sourceMock struct {
	unstructured.Unstructured
	revision string
}

func (m sourceMock) GetRequeueAfter() time.Duration {
	return time.Second * 0
}

func (m *sourceMock) GetArtifact() *sourcev1.Artifact {
	if m.revision != "" {
		return &sourcev1.Artifact{
			Revision: m.revision,
		}
	}
	return nil
}
