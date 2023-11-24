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

func Test_newUninstall(t *testing.T) {
	t.Run("new uninstall", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "uninstall",
				Namespace: "uninstall-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
				Uninstall: &v2.Uninstall{
					Timeout:     &metav1.Duration{Duration: 10 * time.Second},
					KeepHistory: true,
				},
			},
		}

		got := newUninstall(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Timeout).To(Equal(obj.Spec.Uninstall.Timeout.Duration))
		g.Expect(got.KeepHistory).To(Equal(obj.Spec.Uninstall.KeepHistory))
	})

	t.Run("timeout fallback", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "uninstall",
				Namespace: "uninstall-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
			},
		}

		got := newUninstall(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Timeout).To(Equal(obj.Spec.Timeout.Duration))
	})

	t.Run("applies options", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "uninstall",
				Namespace: "uninstall-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		got := newUninstall(&helmaction.Configuration{}, obj, []UninstallOption{
			func(uninstall *helmaction.Uninstall) {
				uninstall.Wait = true
			},
			func(uninstall *helmaction.Uninstall) {
				uninstall.DisableHooks = true
			},
		})
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Wait).To(BeTrue())
		g.Expect(got.DisableHooks).To(BeTrue())
	})
}
