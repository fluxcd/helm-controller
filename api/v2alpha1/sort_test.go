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

package v2alpha1

import (
	"reflect"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDependencySort(t *testing.T) {
	tests := []struct {
		name    string
		ks      []HelmRelease
		want    []HelmRelease
		wantErr bool
	}{
		{
			"simple",
			[]HelmRelease{
				{
					ObjectMeta: v1.ObjectMeta{Name: "frontend"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"backend"}},
				},
				{
					ObjectMeta: v1.ObjectMeta{Name: "common"},
				},
				{
					ObjectMeta: v1.ObjectMeta{Name: "backend"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"common"}},
				},
			},
			[]HelmRelease{
				{
					ObjectMeta: v1.ObjectMeta{Name: "common"},
				},
				{
					ObjectMeta: v1.ObjectMeta{Name: "backend"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"common"}},
				},
				{
					ObjectMeta: v1.ObjectMeta{Name: "frontend"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"backend"}},
				},
			},
			false,
		},
		{
			"circle dependency",
			[]HelmRelease{
				{
					ObjectMeta: v1.ObjectMeta{Name: "dependency"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"endless"}},
				},
				{
					ObjectMeta: v1.ObjectMeta{Name: "endless"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"circular"}},
				},
				{
					ObjectMeta: v1.ObjectMeta{Name: "circular"},
					Spec:       HelmReleaseSpec{DependsOn: []string{"dependency"}},
				},
			},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DependencySort(tt.ks)
			if (err != nil) != tt.wantErr {
				t.Errorf("DependencySort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DependencySort() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDependencySort_DeadEnd(t *testing.T) {
	ks := []HelmRelease{
		{
			ObjectMeta: v1.ObjectMeta{Name: "backend"},
			Spec:       HelmReleaseSpec{DependsOn: []string{"common"}},
		},
		{
			ObjectMeta: v1.ObjectMeta{Name: "frontend"},
			Spec:       HelmReleaseSpec{DependsOn: []string{"infra"}},
		},
		{
			ObjectMeta: v1.ObjectMeta{Name: "common"},
		},
	}
	got, err := DependencySort(ks)
	if err != nil {
		t.Errorf("DependencySort() error = %v", err)
		return
	}
	if len(got) != len(ks) {
		t.Errorf("DependencySort() len = %v, want %v", len(got), len(ks))
	}
}
