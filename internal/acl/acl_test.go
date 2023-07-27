/*
Copyright 2023 The Flux authors

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

package acl

import (
	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestAllowsAccessTo(t *testing.T) {
	tests := []struct {
		name    string
		allow   bool
		obj     client.Object
		ref     types.NamespacedName
		wantErr bool
	}{
		{
			name:  "allow cross-namespace reference",
			allow: true,
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-name",
					Namespace: "some-namespace",
				},
			},
			ref: types.NamespacedName{
				Name:      "some-name",
				Namespace: "some-other-namespace",
			},
			wantErr: false,
		},
		{
			name:  "disallow cross-namespace reference",
			allow: false,
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-name",
					Namespace: "some-namespace",
				},
			},
			ref: types.NamespacedName{
				Name:      "some-name",
				Namespace: "some-other-namespace",
			},
			wantErr: true,
		},
		{
			name:  "allow same-namespace reference",
			allow: false,
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-name",
					Namespace: "some-namespace",
				},
			},
			ref: types.NamespacedName{
				Name:      "some-name",
				Namespace: "some-namespace",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			curAllow := AllowCrossNamespaceRef
			AllowCrossNamespaceRef = tt.allow
			t.Cleanup(func() { AllowCrossNamespaceRef = curAllow })

			if err := AllowsAccessTo(tt.obj, "mock", tt.ref); (err != nil) != tt.wantErr {
				t.Errorf("AllowsAccessTo() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
