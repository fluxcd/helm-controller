/*
Copyright 2026 The Flux authors

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

package postrender

import (
	"bytes"
	"testing"

	. "github.com/onsi/gomega"
)

// helmV4ListUnwrap mimics Helm v4 annotateAndMerge output: a kind:List with
// in-document anchors is unwrapped into separate docs that still carry the
// original &anchor / *alias markers (invalid cross-document YAML).
const helmV4ListUnwrap = `apiVersion: batch/v1
kind: Job
metadata:
  name: example
spec: &jobSpec
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: main
        image: busybox:latest
        command: ["true"]
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: example
spec:
  schedule: "0 0 * * *"
  jobTemplate:
    spec: *jobSpec
`

func Test_inflateYAMLAliases_crossDocFromListUnwrap(t *testing.T) {
	g := NewWithT(t)

	out, err := inflateYAMLAliases([]byte(helmV4ListUnwrap))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(out)).ToNot(ContainSubstring("*jobSpec"))
	g.Expect(string(out)).ToNot(ContainSubstring("&jobSpec"))
	g.Expect(string(out)).To(ContainSubstring("kind: Job"))
	g.Expect(string(out)).To(ContainSubstring("kind: CronJob"))
	// Alias inlined into the CronJob.
	g.Expect(string(out)).To(ContainSubstring("restartPolicy: Never"))
}

func Test_inflateYAMLAliases_unchangedWhenClean(t *testing.T) {
	g := NewWithT(t)

	in := []byte(mixedResourceMock)
	out, err := inflateYAMLAliases(in)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(out).To(Equal(in))
}

func Test_OriginLabels_Run_crossDocAnchorsFromListUnwrap(t *testing.T) {
	g := NewWithT(t)

	// Combined is what BuildPostRenderers returns; it inflates aliases first.
	c := NewCombined(NewOriginLabels("helm.toolkit.fluxcd.io", "namespace", "name"))
	got, err := c.Run(bytes.NewBufferString(helmV4ListUnwrap))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got.String()).To(ContainSubstring("helm.toolkit.fluxcd.io/name: name"))
	g.Expect(got.String()).To(ContainSubstring("kind: CronJob"))
	g.Expect(got.String()).ToNot(ContainSubstring("*jobSpec"))
}

func Test_OriginLabels_Run_crossDocAnchorsWithoutCombined(t *testing.T) {
	g := NewWithT(t)

	// Direct OriginLabels still fails on the raw Helm v4 unwrap (documents the
	// root cause). Combined must be used for the recovery path.
	k := NewOriginLabels("helm.toolkit.fluxcd.io", "namespace", "name")
	_, err := k.Run(bytes.NewBufferString(helmV4ListUnwrap))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unknown anchor"))
}
