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

package storage

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
)

func TestObserver_Name(t *testing.T) {
	g := NewWithT(t)

	o := NewObserver(helmdriver.NewMemory())
	g.Expect(o.Name()).To(Equal(ObserverDriverName))
}

func TestObserver_Get(t *testing.T) {
	t.Run("ignores get", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()
		rel := releaseStub("success", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})

		got, err := o.Get(key)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(rel))
		g.Expect(called).To(BeFalse())
	})
}

func TestObserver_List(t *testing.T) {
	t.Run("ignores list", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()
		rel := releaseStub("success", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})
		got, err := o.List(func(r *helmrelease.Release) bool {
			// Include everything
			return true
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(HaveLen(1))
		g.Expect(got[0]).To(Equal(rel))
		g.Expect(called).To(BeFalse())
	})
}

func TestObserver_Query(t *testing.T) {
	t.Run("ignores query", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()
		rel := releaseStub("success", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})

		rls, err := o.Query(map[string]string{"status": "deployed"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rls).To(HaveLen(1))
		g.Expect(rls[0]).To(Equal(rel))
		g.Expect(called).To(BeFalse())
	})
}

func TestObserver_Create(t *testing.T) {
	t.Run("observes create success", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()
		rel := releaseStub("success", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})

		g.Expect(o.Create(key, rel)).To(Succeed())
		g.Expect(called).To(BeTrue())
	})

	t.Run("ignores create error", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()

		rel := releaseStub("error", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})

		rel2 := releaseStub("error", 1, "ns1", helmrelease.StatusFailed)
		g.Expect(o.Create(key, rel2)).To(HaveOccurred())
		g.Expect(called).To(BeFalse())
	})
}

func TestObserver_Update(t *testing.T) {
	t.Run("observes update success", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()
		rel := releaseStub("success", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})

		g.Expect(o.Update(key, rel)).To(Succeed())
		g.Expect(called).To(BeTrue())
	})

	t.Run("ignores update error", func(t *testing.T) {
		g := NewWithT(t)

		var called bool
		o := NewObserver(helmdriver.NewMemory(), func(rls *helmrelease.Release) {
			called = true
		})

		rel := releaseStub("error", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(o.Update(key, rel)).To(HaveOccurred())
		g.Expect(called).To(BeFalse())
	})
}

func TestObserver_Delete(t *testing.T) {
	t.Run("observes delete success", func(t *testing.T) {
		g := NewWithT(t)

		ms := helmdriver.NewMemory()
		rel := releaseStub("success", 1, "ns1", helmrelease.StatusDeployed)
		key := testKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		var called bool
		o := NewObserver(ms, func(rls *helmrelease.Release) {
			called = true
		})

		got, err := o.Delete(key)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(called).To(BeTrue())

		_, err = ms.Get(key)
		g.Expect(err).To(Equal(helmdriver.ErrReleaseNotFound))
	})

	t.Run("delete release not found", func(t *testing.T) {
		g := NewWithT(t)

		var called bool
		o := NewObserver(helmdriver.NewMemory(), func(rls *helmrelease.Release) {
			called = true
		})

		key := testKey("error", 1)
		got, err := o.Delete(key)
		g.Expect(err).To(Equal(helmdriver.ErrReleaseNotFound))
		g.Expect(got).To(BeNil())
		g.Expect(called).To(BeFalse())
	})
}

func releaseStub(name string, version int, namespace string, status helmrelease.Status) *helmrelease.Release {
	return &helmrelease.Release{
		Name:      name,
		Version:   version,
		Namespace: namespace,
		Info:      &helmrelease.Info{Status: status},
	}
}

func testKey(name string, vers int) string {
	return fmt.Sprintf("%s.v%d", name, vers)
}
