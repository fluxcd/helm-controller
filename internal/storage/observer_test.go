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
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/time"
)

var (
	// smallRelease is 17K while encoded.
	smallRelease *release.Release
	// midRelease is 125K while encoded.
	midRelease *release.Release
	// biggerRelease is 862K while encoded.
	biggerRelease *release.Release
)

func TestMain(m *testing.M) {
	var err error
	if smallRelease, err = decodeReleaseFromFile("testdata/podinfo-helm-1"); err != nil {
		log.Fatal(err)
	}
	if midRelease, err = decodeReleaseFromFile("testdata/istio-base-1"); err != nil {
		log.Fatal(err)
	}
	if biggerRelease, err = decodeReleaseFromFile("testdata/prom-stack-1"); err != nil {
		log.Fatal(err)
	}
	r := m.Run()
	os.Exit(r)
}

func TestObservedRelease_DeepCopyInto(t *testing.T) {
	t.Run("deep copies", func(t *testing.T) {
		g := NewWithT(t)

		now := time.Now()
		in := ObservedRelease{
			Name:    "universe",
			Version: 42,
			Info: release.Info{
				FirstDeployed: now,
				Description:   "ever expanding",
				Status:        release.StatusPendingRollback,
			},
			ChartMetadata: chart.Metadata{
				Name:    "bang",
				Version: "v1.0",
				Maintainers: []*chart.Maintainer{
					{Name: "Lord", Email: "noreply@example.com"},
				},
				Annotations: map[string]string{
					"big": "bang",
				},
				APIVersion: chart.APIVersionV2,
				Type:       "application",
			},
			Config: map[string]interface{}{
				"sky": "blue",
			},
			Manifest: `---
apiVersion: v1
kind: ConfigMap
Namespace: void
data:
  sky: blue
`,
			ManifestSHA256: "1e472606d9e10ab58c5264a6b45aa2d5dad96d06f27423140fd6280a48a0b775",
			Hooks: []release.Hook{
				{
					Name:   "passing-test",
					Events: []release.HookEvent{release.HookTest},
					LastRun: release.HookExecution{
						StartedAt:   now,
						CompletedAt: now,
						Phase:       release.HookPhaseSucceeded,
					},
				},
			},
			Namespace: "void",
			Labels: map[string]string{
				"concept": "true",
			},
		}

		out := ObservedRelease{}
		in.DeepCopyInto(&out)
		g.Expect(out).To(Equal(in))
		g.Expect(out).ToNot(BeIdenticalTo(in))

		deepcopy := out.DeepCopy()
		g.Expect(deepcopy).To(Equal(out))
		g.Expect(deepcopy).ToNot(BeIdenticalTo(out))
	})

	t.Run("with nil", func(t *testing.T) {
		in := ObservedRelease{}
		in.DeepCopyInto(nil)
	})
}

func TestNewObservedRelease(t *testing.T) {
	tests := []struct {
		name     string
		releases []*release.Release
		inspect  func(w *WithT, rel *release.Release, obsRel ObservedRelease)
	}{
		{
			name:     "observes release",
			releases: []*release.Release{smallRelease, midRelease, biggerRelease},
			inspect: func(w *WithT, rel *release.Release, obsRel ObservedRelease) {
				w.Expect(obsRel.Name).To(Equal(rel.Name))
				w.Expect(obsRel.Version).To(Equal(rel.Version))
				w.Expect(obsRel.Info).To(Equal(*rel.Info))
				w.Expect(obsRel.ChartMetadata).To(Equal(*rel.Chart.Metadata))
				w.Expect(obsRel.Config).To(Equal(rel.Config))
				w.Expect(obsRel.Manifest).To(Equal(rel.Manifest))
				w.Expect(obsRel.ManifestSHA256).To(Equal(fmt.Sprintf("%x", sha256.Sum256([]byte(rel.Manifest)))))
				w.Expect(obsRel.Hooks).To(HaveLen(len(rel.Hooks)))
				for k, v := range rel.Hooks {
					w.Expect(obsRel.Hooks[k]).To(Equal(*v))
				}
				w.Expect(obsRel.Namespace).To(Equal(rel.Namespace))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, rel := range tt.releases {
				rel := rel
				t.Run(t.Name()+"_"+rel.Name, func(t *testing.T) {
					got := NewObservedRelease(rel)
					tt.inspect(NewWithT(t), rel, got)
				})
			}
		})
	}
}

func TestObserver_Name(t *testing.T) {
	g := NewWithT(t)

	o := NewObserver(driver.NewMemory())
	g.Expect(o.Name()).To(Equal(ObserverDriverName))
}

func TestObserver_Get(t *testing.T) {
	t.Run("ignores get", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())
		g.Expect(o.releases).To(HaveLen(0))

		got, err := o.Get(key)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(rel))
		g.Expect(o.releases).To(HaveLen(0))
	})
}

func TestObserver_List(t *testing.T) {
	t.Run("ignores list", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := makeKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		o := NewObserver(ms)
		got, err := o.List(func(r *release.Release) bool {
			// Include everything
			return true
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(HaveLen(1))
		g.Expect(got[0]).To(Equal(rel))
		// Observed releases still empty
		g.Expect(o.releases).To(HaveLen(0))
	})
}

func TestObserver_Query(t *testing.T) {
	t.Run("ignores query", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := makeKey(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		o := NewObserver(ms)
		rls, err := o.Query(map[string]string{"status": "deployed"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rls).To(HaveLen(1))
		g.Expect(rls[0]).To(Equal(rel))
		// Observed releases still empty
		g.Expect(o.releases).To(HaveLen(0))
	})
}

func TestObserver_Create(t *testing.T) {
	t.Run("observes create success", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Create(key, rel)).To(Succeed())
		g.Expect(o.releases).To(HaveLen(1))
		g.Expect(o.releases).To(HaveKey(key))
		g.Expect(o.releases[key]).To(Equal(NewObservedRelease(rel)))
	})

	t.Run("ignores create error", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("error", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Create(key, rel)).To(Succeed())

		rel2 := releaseStub("error", 1, "ns1", release.StatusFailed)
		g.Expect(o.Create(key, rel2)).To(HaveOccurred())
		g.Expect(o.releases).To(HaveLen(1))
		g.Expect(o.releases).To(HaveKey(key))
		g.Expect(o.releases[key]).ToNot(Equal(rel2))
	})
}

func TestObserver_Update(t *testing.T) {
	t.Run("observes update success", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(ms.Create(key, rel)).To(Succeed())

		g.Expect(o.Update(key, rel)).To(Succeed())
		g.Expect(o.releases).To(HaveLen(1))
		g.Expect(o.releases).To(HaveKey(key))
		g.Expect(o.releases[key]).To(Equal(NewObservedRelease(rel)))
	})

	t.Run("observation updates earlier observation", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Create(key, rel)).To(Succeed())

		rel2 := releaseStub("success", 1, "ns1", release.StatusFailed)
		g.Expect(o.Update(key, rel2)).To(Succeed())
		g.Expect(o.releases[key]).To(Equal(NewObservedRelease(rel2)))
	})

	t.Run("ignores update error", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("error", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Update(key, rel)).To(HaveOccurred())
		g.Expect(o.releases).To(HaveLen(0))
	})
}

func TestObserver_Delete(t *testing.T) {
	t.Run("observes delete success", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		rel := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Create(key, rel)).To(Succeed())
		g.Expect(o.LastObservation(rel.Name)).ToNot(BeNil())

		got, err := o.Delete(key)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())

		g.Expect(o.releases).To(HaveLen(1))
		g.Expect(o.releases).To(HaveKey(key))
		g.Expect(o.releases[key]).To(Equal(NewObservedRelease(got)))

		_, err = ms.Get(key)
		g.Expect(err).To(Equal(driver.ErrReleaseNotFound))
	})

	t.Run("delete release not found", func(t *testing.T) {
		g := NewWithT(t)

		ms := driver.NewMemory()
		o := NewObserver(ms)

		key := o.makeKeyFunc("error", 1)
		got, err := o.Delete(key)
		g.Expect(err).To(Equal(driver.ErrReleaseNotFound))
		g.Expect(got).To(BeNil())
	})
}

func TestObserver_LastObservation(t *testing.T) {
	t.Run("last observation by version", func(t *testing.T) {
		g := NewWithT(t)

		o := NewObserver(driver.NewMemory())

		rel1 := releaseStub("success", 1, "ns1", release.StatusDeployed)
		key1 := o.makeKeyFunc(rel1.Name, rel1.Version)

		rel2 := releaseStub("success", 2, "ns1", release.StatusDeployed)
		key2 := o.makeKeyFunc(rel2.Name, rel2.Version)

		g.Expect(o.Create(key2, rel2)).To(Succeed())
		g.Expect(o.Create(key1, rel1)).To(Succeed())

		got, err := o.LastObservation(rel2.Name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(NewObservedRelease(rel2)))
	})

	t.Run("no observed releases", func(t *testing.T) {
		g := NewWithT(t)

		o := NewObserver(driver.NewMemory())
		got, err := o.LastObservation("notobserved")
		g.Expect(err).To(Equal(ErrReleaseNotObserved))
		g.Expect(got).To(Equal(ObservedRelease{}))
	})

	t.Run("no observed releases for name", func(t *testing.T) {
		g := NewWithT(t)

		o := NewObserver(driver.NewMemory())

		otherRel := releaseStub("other", 2, "ns1", release.StatusDeployed)
		otherKey := o.makeKeyFunc(otherRel.Name, otherRel.Version)
		g.Expect(o.Create(otherKey, otherRel)).To(Succeed())

		got, err := o.LastObservation("notobserved")
		g.Expect(err).To(Equal(ErrReleaseNotObserved))
		g.Expect(got).To(Equal(ObservedRelease{}))
	})
}

func TestObserver_GetObservedVersion(t *testing.T) {
	t.Run("observation with version", func(t *testing.T) {
		g := NewWithT(t)

		o := NewObserver(driver.NewMemory())

		rel := releaseStub("thirtythree", 33, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Create(key, rel)).To(Succeed())

		got, err := o.GetObservedVersion(rel.Name, rel.Version)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(NewObservedRelease(rel)))
	})

	t.Run("unobserved version", func(t *testing.T) {
		g := NewWithT(t)

		o := NewObserver(driver.NewMemory())

		rel := releaseStub("two", 2, "ns1", release.StatusDeployed)
		key := o.makeKeyFunc(rel.Name, rel.Version)
		g.Expect(o.Create(key, rel)).To(Succeed())

		got, err := o.GetObservedVersion("two", 1)
		g.Expect(err).To(Equal(ErrReleaseNotObserved))
		g.Expect(got).To(Equal(ObservedRelease{}))
	})
}

func TestObserver_ObserveLastRelease(t *testing.T) {
	t.Run("observes last release from storage", func(t *testing.T) {
		g := NewWithT(t)

		d := driver.NewMemory()

		rel1 := releaseStub("two", 1, "ns1", release.StatusDeployed)
		key1 := makeKey(rel1.Name, rel1.Version)
		g.Expect(d.Create(key1, rel1)).To(Succeed())

		rel2 := releaseStub("two", 2, "ns1", release.StatusDeployed)
		key2 := makeKey(rel2.Name, rel2.Version)
		g.Expect(d.Create(key2, rel2)).To(Succeed())

		o := NewObserver(d)
		got, err := o.ObserveLastRelease("two")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(NewObservedRelease(rel2)))
	})

	t.Run("error on release not found", func(t *testing.T) {
		g := NewWithT(t)

		o := NewObserver(driver.NewMemory())
		got, err := o.ObserveLastRelease("notfound")
		g.Expect(err).To(Equal(driver.ErrReleaseNotFound))
		g.Expect(got).To(Equal(ObservedRelease{}))
	})
}

func Test_makeKey(t *testing.T) {
	tests := []struct {
		name    string
		version int
		want    string
	}{
		{name: "release-a", version: 2, want: "sh.helm.release.v1.release-a.v2"},
		{name: "release-b", version: 48, want: "sh.helm.release.v1.release-b.v48"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%d", tt.name, tt.version), func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(makeKey(tt.name, tt.version)).To(Equal(tt.want))
		})
	}
}

func Test_splitKey(t *testing.T) {
	tests := []struct {
		key         string
		wantName    string
		wantVersion int
	}{
		{key: "sh.helm.release.v1.release-a.v2", wantName: "release-a", wantVersion: 2},
		{key: "sh.helm.release.v1.release-b.v48", wantName: "release-b", wantVersion: 48},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			g := NewWithT(t)

			gotN, gotV := splitKey(tt.key)
			g.Expect(gotN).To(Equal(tt.wantName))
			g.Expect(gotV).To(Equal(tt.wantVersion))
		})
	}
}

func Test_makeKey_splitKey(t *testing.T) {
	g := NewWithT(t)

	key := makeKey("release-name", 894)
	gotN, gotV := splitKey(key)
	g.Expect(gotN).To(Equal("release-name"))
	g.Expect(gotV).To(Equal(894))
}

func releaseStub(name string, version int, namespace string, status release.Status) *release.Release {
	return &release.Release{
		Name:      name,
		Version:   version,
		Namespace: namespace,
		Info:      &release.Info{Status: status},
	}
}

func decodeReleaseFromFile(path string) (*release.Release, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load encoded release data: %w", err)
	}
	rel, err := decodeRelease(string(b))
	if err != nil {
		return nil, fmt.Errorf("failed to decode release data: %w", err)
	}
	return rel, nil
}

func benchmarkNewObservedRelease(rel release.Release, b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		NewObservedRelease(&rel)
	}
}

func BenchmarkNewObservedReleaseSmall(b *testing.B) {
	benchmarkNewObservedRelease(*smallRelease, b)
}

func BenchmarkNewObservedReleaseMid(b *testing.B) {
	benchmarkNewObservedRelease(*midRelease, b)
}

func BenchmarkNewObservedReleaseBigger(b *testing.B) {
	benchmarkNewObservedRelease(*biggerRelease, b)
}
