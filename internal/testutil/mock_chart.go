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

package testutil

import (
	"fmt"

	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"
)

var manifestTmpl = `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
  namespace: %[1]s
data:
  foo: bar
`

var manifestWithCustomNameTmpl = `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-%[1]s
  namespace: %[2]s
data:
  foo: bar
`

var manifestWithHookTmpl = `apiVersion: v1
kind: ConfigMap
metadata:
  name: hook
  namespace: %[1]s
  annotations:
    "helm.sh/hook": post-install,pre-delete,post-upgrade
data:
  name: value
`

var manifestWithFailingHookTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: failing-hook
  namespace: %[1]s
  annotations:
    "helm.sh/hook": post-install,pre-delete,post-upgrade
spec:
  containers:
  - name: test
    image: alpine
    command: ["/bin/sh", "-c", "exit 1"]
`

var manifestWithTestHookTmpl = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-hook
  namespace: %[1]s
  annotations:
    "helm.sh/hook": test
data:
  test: data
`

var manifestWithFailingTestHookTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: failing-test-hook
  namespace: %[1]s
  annotations:
    "helm.sh/hook": test
spec:
  containers:
  - name: test
    image: alpine
    command: ["/bin/sh", "-c", "exit 1"]
  restartPolicy: Never
`

var crdManifest = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: crontabs.stable.example.com
spec:
  group: stable.example.com
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                cronSpec:
                  type: string
                image:
                  type: string
                replicas:
                  type: integer
  scope: Namespaced
  names:
    plural: crontabs
    singular: crontab
    kind: CronTab
    shortNames:
    - ct
`

// ChartOptions is a helper to build a Helm chart object.
type ChartOptions struct {
	*helmchart.Chart
}

// ChartOption is a function that can be used to modify a chart.
type ChartOption func(*ChartOptions)

// BuildChart returns a Helm chart object built with basic data
// and any provided chart options.
func BuildChart(opts ...ChartOption) *helmchart.Chart {
	c := &ChartOptions{
		Chart: &helmchart.Chart{
			// TODO: This should be more complete.
			Metadata: &helmchart.Metadata{
				APIVersion: "v1",
				Name:       "hello",
				Version:    "0.1.0",
				AppVersion: "1.2.3",
			},
			// This adds a basic template and hooks.
			Templates: []*helmchart.File{
				{
					Name: "templates/manifest",
					Data: []byte(fmt.Sprintf(manifestTmpl, "{{ default .Release.Namespace }}")),
				},
				{
					Name: "templates/hooks",
					Data: []byte(fmt.Sprintf(manifestWithHookTmpl, "{{ default .Release.Namespace }}")),
				},
			},
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c.Chart
}

// ChartWithName sets the name of the chart.
func ChartWithName(name string) ChartOption {
	return func(opts *ChartOptions) {
		opts.Metadata.Name = name
	}
}

// ChartWithVersion sets the version of the chart.
func ChartWithVersion(version string) ChartOption {
	return func(opts *ChartOptions) {
		opts.Metadata.Version = version
	}
}

// ChartWithFailingHook appends a failing hook to the chart.
func ChartWithFailingHook() ChartOption {
	return func(opts *ChartOptions) {
		opts.Templates = append(opts.Templates, &helmchart.File{
			Name: "templates/failing-hook",
			Data: []byte(fmt.Sprintf(manifestWithFailingHookTmpl, "{{ default .Release.Namespace }}")),
		})
	}
}

// ChartWithTestHook appends a test hook to the chart.
func ChartWithTestHook() ChartOption {
	return func(opts *ChartOptions) {
		opts.Templates = append(opts.Templates, &helmchart.File{
			Name: "templates/test-hooks",
			Data: []byte(fmt.Sprintf(manifestWithTestHookTmpl, "{{ default .Release.Namespace }}")),
		})
	}
}

// ChartWithFailingTestHook appends a failing test hook to the chart.
func ChartWithFailingTestHook() ChartOption {
	return func(options *ChartOptions) {
		options.Templates = append(options.Templates, &helmchart.File{
			Name: "templates/test-hooks",
			Data: []byte(fmt.Sprintf(manifestWithFailingTestHookTmpl, "{{ default .Release.Namespace }}")),
		})
	}
}

// ChartWithManifestWithCustomName sets the name of the manifest.
func ChartWithManifestWithCustomName(name string) ChartOption {
	return func(opts *ChartOptions) {
		opts.Templates = []*helmchart.File{
			{
				Name: "templates/manifest",
				Data: []byte(fmt.Sprintf(manifestWithCustomNameTmpl, name, "{{ default .Release.Namespace }}")),
			},
		}
	}
}

// ChartWithCRD appends a CRD to the chart.
func ChartWithCRD() ChartOption {
	return func(opts *ChartOptions) {
		opts.Files = []*helmchart.File{
			{
				Name: "crds/crd.yaml",
				Data: []byte(crdManifest),
			},
		}
	}
}

// ChartWithDependency appends a dependency to the chart.
func ChartWithDependency(md *helmchart.Dependency, chrt *helmchart.Chart) ChartOption {
	return func(opts *ChartOptions) {
		opts.Metadata.Dependencies = append(opts.Metadata.Dependencies, md)
		opts.AddDependency(chrt)
	}
}

// ChartWithValues sets the values.yaml file of the chart.
func ChartWithValues(values map[string]any) ChartOption {
	return func(opts *ChartOptions) {
		opts.Values = values
	}
}

// BuildChartWithSubchartWithCRD returns a Helm chart object with a subchart
// that contains a CRD. Useful for testing helm-controller's staged CRDs-first
// deployment logic.
func BuildChartWithSubchartWithCRD() *helmchart.Chart {
	subChart := BuildChart(
		ChartWithName("subchart"),
		ChartWithManifestWithCustomName("sub-chart"),
		ChartWithCRD(),
		ChartWithValues(helmchartutil.Values{
			"foo":     "bar",
			"exports": map[string]any{"data": map[string]any{"myint": 123}},
			"default": map[string]any{"data": map[string]any{"myint": 456}},
		}))
	mainChart := BuildChart(
		ChartWithManifestWithCustomName("main-chart"),
		ChartWithValues(helmchartutil.Values{
			"foo":       "baz",
			"myimports": map[string]any{"myint": 0},
		}),
		ChartWithDependency(&helmchart.Dependency{
			Name:      "subchart",
			Condition: "subchart.enabled",
			ImportValues: []any{
				"data",
				map[string]any{
					"child":  "default.data",
					"parent": "myimports",
				},
			},
		}, subChart))
	return mainChart
}
