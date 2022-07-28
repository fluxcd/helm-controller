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

package action

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	helmaction "helm.sh/helm/v3/pkg/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

func Test_newTest(t *testing.T) {
	t.Run("new test", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
				Test: &v2.Test{
					Timeout: &metav1.Duration{Duration: 10 * time.Second},
					Filters: &[]v2.Filter{
						{
							Name: "test",
						},
						{
							Name:    "test2",
							Exclude: true,
						},
					},
				},
			},
		}

		got := newTest(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Namespace).To(Equal(obj.Namespace))
		g.Expect(got.Timeout).To(Equal(obj.Spec.Test.Timeout.Duration))
		g.Expect(got.Filters).To(HaveLen(2))
		g.Expect(got.Filters).To(HaveKeyWithValue(Equal("name"), ContainElement("test")))
		g.Expect(got.Filters).To(HaveKeyWithValue(Equal("!name"), ContainElement("test2")))
	})

	t.Run("timeout fallback", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
			},
		}

		got := newTest(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Namespace).To(Equal(obj.Namespace))
		g.Expect(got.Timeout).To(Equal(obj.Spec.Timeout.Duration))
	})

	t.Run("applies options", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		got := newTest(&helmaction.Configuration{}, obj, []TestOption{
			func(test *helmaction.ReleaseTesting) {
				test.Filters = map[string][]string{
					"test": {"test"},
				}
			},
			func(test *helmaction.ReleaseTesting) {
				test.Filters["test2"] = []string{"test2"}
			},
		})
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Filters).To(HaveLen(2))
	})
}
