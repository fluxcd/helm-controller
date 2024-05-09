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

package release

import (
	"encoding/json"
	"io"

	"github.com/mitchellh/copystructure"
	"helm.sh/helm/v3/pkg/chart"
	helmrelease "helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/digest"
)

var (
	DefaultDataFilters = []DataFilter{
		IgnoreHookTestEvents,
	}
)

// DataFilter allows for filtering data from the returned Observation while
// making an observation.
type DataFilter func(rel *Observation)

// IgnoreHookTestEvents ignores test event hooks. For example, to exclude it
// while generating a digest for the object. To prevent manual test triggers
// from a user to interfere with the checksum.
func IgnoreHookTestEvents(rel *Observation) {
	if len(rel.Hooks) > 0 {
		var hooks []helmrelease.Hook
		for i := range rel.Hooks {
			h := rel.Hooks[i]
			if !IsHookForEvent(&h, helmrelease.HookTest) {
				hooks = append(hooks, h)
			}
		}
		rel.Hooks = hooks
	}
}

// Observation is a copy of a Helm release object, as observed to be written
// to the storage by a storage.Observer. The object is detached from the Helm
// storage object, and mutations to it do not change the underlying release
// object.
type Observation struct {
	// Name of the release.
	Name string `json:"name"`
	// Version of the release, at times also called revision.
	Version int `json:"version"`
	// Info provides information about the release.
	Info helmrelease.Info `json:"info"`
	// ChartMetadata contains the current Chartfile data of the release.
	ChartMetadata chart.Metadata `json:"chartMetadata"`
	// Config is the set of extra Values added to the chart.
	// These values override the default values inside the chart.
	Config map[string]interface{} `json:"config"`
	// Manifest is the string representation of the rendered template.
	Manifest string `json:"manifest"`
	// Hooks are all the hooks declared for this release, and the current
	// state they are in.
	Hooks []helmrelease.Hook `json:"hooks"`
	// Namespace is the Kubernetes namespace of the release.
	Namespace string `json:"namespace"`
	// OCIDigest is the digest of the OCI artifact that was used to
	OCIDigest string `json:"ociDigest,omitempty"`
}

// Targets returns if the release matches the given name, namespace and
// version. If the version is 0, it matches any version.
func (o Observation) Targets(name, namespace string, version int) bool {
	return o.Name == name && o.Namespace == namespace && (version == 0 || o.Version == version)
}

// Encode JSON encodes the Observation and writes it into the given writer.
func (o Observation) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(o); err != nil {
		return err
	}
	return nil
}

// ObserveRelease deep copies the values from the provided release.Release
// into a new Observation while omitting all chart data except metadata.
// If no filters are provided, it defaults to DefaultDataFilters. To not use
// any filters, pass an explicit empty slice.
func ObserveRelease(rel *helmrelease.Release, filter ...DataFilter) Observation {
	if rel == nil {
		return Observation{}
	}

	if filter == nil {
		filter = DefaultDataFilters
	}

	obsRel := Observation{
		Name:      rel.Name,
		Version:   rel.Version,
		Config:    nil,
		Manifest:  rel.Manifest,
		Hooks:     nil,
		Namespace: rel.Namespace,
	}

	if rel.Info != nil {
		obsRel.Info = *rel.Info
	}

	if rel.Chart != nil && rel.Chart.Metadata != nil {
		if v, err := copystructure.Copy(rel.Chart.Metadata); err == nil {
			obsRel.ChartMetadata = *v.(*chart.Metadata)
		}
	}

	if len(rel.Config) > 0 {
		if v, err := copystructure.Copy(rel.Config); err == nil {
			obsRel.Config = v.(map[string]interface{})
		}
	}

	if len(rel.Hooks) > 0 {
		obsRel.Hooks = make([]helmrelease.Hook, len(rel.Hooks))
		if v, err := copystructure.Copy(rel.Hooks); err == nil {
			for i, h := range v.([]*helmrelease.Hook) {
				obsRel.Hooks[i] = *h
			}
		}
	}

	for _, f := range filter {
		f(&obsRel)
	}

	return obsRel
}

// ObservedToSnapshot returns a v2.Snapshot constructed from the
// Observation data. Calculating the (config) digest using the
// digest.Canonical algorithm.
func ObservedToSnapshot(rls Observation) *v2.Snapshot {
	return &v2.Snapshot{
		Digest:        Digest(digest.Canonical, rls).String(),
		Name:          rls.Name,
		Namespace:     rls.Namespace,
		Version:       rls.Version,
		AppVersion:    rls.ChartMetadata.AppVersion,
		ChartName:     rls.ChartMetadata.Name,
		ChartVersion:  rls.ChartMetadata.Version,
		ConfigDigest:  chartutil.DigestValues(digest.Canonical, rls.Config).String(),
		FirstDeployed: metav1.NewTime(rls.Info.FirstDeployed.Time),
		LastDeployed:  metav1.NewTime(rls.Info.LastDeployed.Time),
		Deleted:       metav1.NewTime(rls.Info.Deleted.Time),
		Status:        rls.Info.Status.String(),
		OCIDigest:     rls.OCIDigest,
	}
}

// TestHooksFromRelease returns the list of v2.TestHookStatus for the
// given release, indexed by name.
func TestHooksFromRelease(rls *helmrelease.Release) map[string]*v2.TestHookStatus {
	hooks := make(map[string]*v2.TestHookStatus)
	for k, v := range GetTestHooks(rls) {
		var h *v2.TestHookStatus
		if v != nil {
			h = &v2.TestHookStatus{
				LastStarted:   metav1.NewTime(v.LastRun.StartedAt.Time),
				LastCompleted: metav1.NewTime(v.LastRun.CompletedAt.Time),
				Phase:         v.LastRun.Phase.String(),
			}
		}
		hooks[k] = h
	}
	return hooks
}
