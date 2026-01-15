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
	helmaction "helm.sh/helm/v4/pkg/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

func Test_newRollback(t *testing.T) {
	t.Run("new rollback", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rollback",
				Namespace: "rollback-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
				Rollback: &v2.Rollback{
					Timeout: &metav1.Duration{Duration: 10 * time.Second},
					Force:   true,
				},
			},
		}

		got := newRollback(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Timeout).To(Equal(obj.Spec.Rollback.Timeout.Duration))
		g.Expect(got.ForceReplace).To(Equal(obj.Spec.Rollback.Force))
		g.Expect(got.MaxHistory).To(Equal(obj.GetMaxHistory()))
	})

	t.Run("rollback to version", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rollback",
				Namespace: "rollback-ns",
			},
		}

		toVersion := 3
		got := newRollback(&helmaction.Configuration{}, obj, []RollbackOption{RollbackToVersion(toVersion)})
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Version).To(Equal(toVersion))
	})

	t.Run("timeout fallback", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rollback",
				Namespace: "rollback-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
			},
		}

		got := newRollback(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Timeout).To(Equal(obj.Spec.Timeout.Duration))
	})

	t.Run("applies options", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rollback",
				Namespace: "rollback-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		got := newRollback(&helmaction.Configuration{}, obj, []RollbackOption{
			func(rollback *helmaction.Rollback) {
				rollback.CleanupOnFail = true
			},
			func(rollback *helmaction.Rollback) {
				rollback.DryRunStrategy = helmaction.DryRunClient
			},
		})
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.CleanupOnFail).To(BeTrue())
		g.Expect(got.DryRunStrategy).To(Equal(helmaction.DryRunClient))
	})

	t.Run("server side apply is auto regardless of UseHelm3Defaults", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rollback",
				Namespace: "rollback-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		// Save and restore UseHelm3Defaults
		oldUseHelm3Defaults := UseHelm3Defaults
		t.Cleanup(func() { UseHelm3Defaults = oldUseHelm3Defaults })

		// Test with UseHelm3Defaults = false
		UseHelm3Defaults = false
		got := newRollback(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("auto"))

		// Test with UseHelm3Defaults = true
		UseHelm3Defaults = true
		got = newRollback(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("auto"))
	})

	t.Run("server side apply user specified", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rollback",
				Namespace: "rollback-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Rollback: &v2.Rollback{
					ServerSideApply: v2.ServerSideApplyEnabled,
				},
			},
		}

		got := newRollback(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("true"))

		obj.Spec.Rollback.ServerSideApply = v2.ServerSideApplyDisabled
		got = newRollback(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("false"))
	})
}
