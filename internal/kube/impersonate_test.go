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

package kube

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

func TestSetImpersonationConfig(t *testing.T) {
	t.Run("DefaultServiceAccountName", func(t *testing.T) {
		g := NewWithT(t)

		DefaultServiceAccountName = "default"
		namespace := "test"
		expect := "system:serviceaccount:" + namespace + ":" + DefaultServiceAccountName

		cfg := &rest.Config{}
		name := SetImpersonationConfig(cfg, namespace, "")
		g.Expect(name).To(Equal(expect))
		g.Expect(cfg.Impersonate.UserName).ToNot(BeEmpty())
		g.Expect(cfg.Impersonate.UserName).To(Equal(name))
	})

	t.Run("overwrite DefaultServiceAccountName", func(t *testing.T) {
		g := NewWithT(t)

		DefaultServiceAccountName = "default"
		namespace := "test"
		serviceAccount := "different"
		expect := "system:serviceaccount:" + namespace + ":" + serviceAccount

		cfg := &rest.Config{}
		name := SetImpersonationConfig(cfg, namespace, serviceAccount)
		g.Expect(name).To(Equal(expect))
		g.Expect(cfg.Impersonate.UserName).ToNot(BeEmpty())
		g.Expect(cfg.Impersonate.UserName).To(Equal(name))
	})

	t.Run("without namespace", func(t *testing.T) {
		g := NewWithT(t)

		serviceAccount := "account"

		cfg := &rest.Config{}
		name := SetImpersonationConfig(cfg, "", serviceAccount)
		g.Expect(name).To(BeEmpty())
		g.Expect(cfg.Impersonate.UserName).To(BeEmpty())
	})

	t.Run("no arguments", func(t *testing.T) {
		g := NewWithT(t)

		cfg := &rest.Config{}
		name := SetImpersonationConfig(cfg, "", "")
		g.Expect(name).To(BeEmpty())
		g.Expect(cfg.Impersonate.UserName).To(BeEmpty())
	})
}
