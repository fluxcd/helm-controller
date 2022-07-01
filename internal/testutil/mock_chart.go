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
)

var manifestTmpl = `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
  namespace: %[1]s
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
