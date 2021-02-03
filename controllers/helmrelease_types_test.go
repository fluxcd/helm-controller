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

package controllers

import (
	"encoding/json"
	"testing"

	"github.com/fluxcd/pkg/apis/kustomize"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestHelmReleaseTypes_unmarshal_PatchJSON6902(t *testing.T) {
	var p kustomize.JSON6902Patch
	err := json.Unmarshal([]byte(`{"target": {"namespace": "ns", "name": "x", "kind": "k", "version": "v"},"patch": [{"op": "add", "path": "/some/new/path", "value": "value"}]}`), &p)
	if err != nil {
		t.Error(err)
	}
	if p.Target.Kind != "k" {
		t.Logf("Invalid Kind: epected 'k' got %s", p.Target.Kind)
		t.Fail()
	}
	if p.Target.Version != "v" {
		t.Logf("Invalid Version: epected 'v' got %s", p.Target.Version)
		t.Fail()
	}
	if p.Target.Name != "x" {
		t.Logf("Invalid Name: epected 'x got %s", p.Target.Name)
		t.Fail()
	}
	if p.Target.Namespace != "ns" {
		t.Logf("Invalid Namespace: epected 'ns' got %s", p.Target.Namespace)
		t.Fail()
	}
	if len(p.Patch) != 1 {
		t.Logf("Failed to unmarshal Patch: got %s", p.Patch)
		t.Fail()
	}
}

// Ensure the generic JSON fields are unmarshaled.
func TestHelmReleaseTypes_unmarshal_Kustomize(t *testing.T) {
	var p v2.Kustomize
	err := json.Unmarshal([]byte(`{"patchesStrategicMerge": [{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}]}`), &p)
	if err != nil {
		t.Error(err)
	}
	if len(p.PatchesStrategicMerge) != 1 {
		t.Logf("Failed to unmarshal PatchesStrategicMerge: got %s", p.PatchesStrategicMerge)
		t.Fail()
	} else {
		sm := p.PatchesStrategicMerge[0]
		s, err := json.Marshal(sm)
		if err != nil {
			t.Error(err)
		}
		var m map[string]interface{}
		err = json.Unmarshal(s, &m)
		if err != nil {
			t.Error(err)
		}
		if m["apiVersion"] != "apps/v1" {
			t.Logf("expected 'apps/v1' got %s", m["apiVersion"])
			t.Fail()
		}
	}
}
