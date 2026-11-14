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
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"helm.sh/helm/v4/pkg/action"
	helmaction "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	helmkube "helm.sh/helm/v4/pkg/kube"
	helmkubefake "helm.sh/helm/v4/pkg/kube/fake"
	helmbaserelease "helm.sh/helm/v4/pkg/release"
	helmreleasecommon "helm.sh/helm/v4/pkg/release/common"
	helmrelease "helm.sh/helm/v4/pkg/release/v1"
	helmstorage "helm.sh/helm/v4/pkg/storage"
	helmdriver "helm.sh/helm/v4/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

// cancelOnCreateDriver cancels the reconcile context after Helm has stored a
// new release revision.
type cancelOnCreateDriver struct {
	helmdriver.Driver
	cancel context.CancelFunc
}

func (d *cancelOnCreateDriver) Create(key string, rls helmbaserelease.Releaser) error {
	if err := d.Driver.Create(key, rls); err != nil {
		return err
	}
	d.cancel()
	return nil
}

// blockingUpgradeClient keeps the resource update in flight until the test
// explicitly allows it to finish.
type blockingUpgradeClient struct {
	*helmkubefake.PrintingKubeClient
	started   chan struct{}
	unblock   chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func (c *blockingUpgradeClient) Update(_, _ helmkube.ResourceList, _ ...helmkube.ClientUpdateOption) (*helmkube.Result, error) {
	c.startOnce.Do(func() { close(c.started) })
	<-c.unblock
	return &helmkube.Result{}, errBlockingUpgradeClient
}

// errBlockingUpgradeClient is returned by blockingUpgradeClient.Update. It
// is distinct from the context errors, to tell the result of the upgrade
// goroutine apart from the one of Helm's context handler.
var errBlockingUpgradeClient = errors.New("resource update failed")

func (c *blockingUpgradeClient) stop() {
	c.stopOnce.Do(func() { close(c.unblock) })
}

func TestUpgrade_WaitsForInFlightActionAfterCancellation(t *testing.T) {
	g := NewWithT(t)

	const (
		releaseName      = "cancelled-upgrade"
		releaseNamespace = "default"
	)

	driver := helmdriver.NewMemory()
	driver.SetNamespace(releaseNamespace)
	store := helmstorage.Init(driver)
	previous := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Version:   1,
		Status:    helmreleasecommon.StatusDeployed,
		Chart:     testutil.BuildChart(),
	})
	g.Expect(store.Create(previous)).To(Succeed())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := &blockingUpgradeClient{
		PrintingKubeClient: &helmkubefake.PrintingKubeClient{Out: io.Discard, LogOutput: io.Discard},
		started:            make(chan struct{}),
		unblock:            make(chan struct{}),
	}
	t.Cleanup(client.stop)

	cfg := helmaction.NewConfiguration()
	cfg.Capabilities = common.DefaultCapabilities
	cfg.KubeClient = client
	cfg.Releases = helmstorage.Init(&cancelOnCreateDriver{Driver: driver, cancel: cancel})
	cfg.HookOutputFunc = func(_, _, _ string) io.Writer { return io.Discard }

	obj := &v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{Name: releaseName, Namespace: releaseNamespace},
		Spec: v2.HelmReleaseSpec{
			ReleaseName:     releaseName,
			TargetNamespace: releaseNamespace,
			Timeout:         &metav1.Duration{Duration: time.Second},
			Upgrade:         &v2.Upgrade{DisableHooks: true},
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := Upgrade(ctx, cfg, obj, testutil.BuildChart(), nil)
		done <- err
	}()

	select {
	case <-client.started:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the Helm resource update to start")
	}

	select {
	case err := <-done:
		t.Fatalf("upgrade returned before its resource update completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	client.stop()
	select {
	case err := <-done:
		g.Expect(errors.Is(err, errBlockingUpgradeClient)).To(BeTrue())
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the Helm upgrade to finish")
	}
}

func TestUpgrade_DoesNotStartWithCanceledContext(t *testing.T) {
	g := NewWithT(t)

	const (
		releaseName      = "cancelled-upgrade"
		releaseNamespace = "default"
	)

	driver := helmdriver.NewMemory()
	driver.SetNamespace(releaseNamespace)
	store := helmstorage.Init(driver)
	g.Expect(store.Create(testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Version:   1,
		Status:    helmreleasecommon.StatusDeployed,
		Chart:     testutil.BuildChart(),
	}))).To(Succeed())

	cfg := helmaction.NewConfiguration()
	cfg.Capabilities = common.DefaultCapabilities
	cfg.KubeClient = &helmkubefake.PrintingKubeClient{Out: io.Discard, LogOutput: io.Discard}
	cfg.Releases = store
	cfg.HookOutputFunc = func(_, _, _ string) io.Writer { return io.Discard }

	obj := &v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{Name: releaseName, Namespace: releaseNamespace},
		Spec: v2.HelmReleaseSpec{
			ReleaseName:     releaseName,
			TargetNamespace: releaseNamespace,
			Timeout:         &metav1.Duration{Duration: time.Second},
			Upgrade:         &v2.Upgrade{DisableHooks: true},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Upgrade(ctx, cfg, obj, testutil.BuildChart(), nil)
	g.Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	history, err := store.History(releaseName)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(history).To(HaveLen(1))
}

func Test_newUpgrade(t *testing.T) {
	t.Run("new upgrade", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
				Upgrade: &v2.Upgrade{
					Timeout: &metav1.Duration{Duration: 10 * time.Second},
					Force:   true,
				},
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Namespace).To(Equal(obj.Namespace))
		g.Expect(got.Timeout).To(Equal(obj.Spec.Upgrade.Timeout.Duration))
		// ForceReplace is not set in the constructor; it is set after SSA resolution
		// in Upgrade() to avoid the Helm SDK mutual exclusivity error.
		g.Expect(got.ForceReplace).To(BeFalse())
	})

	t.Run("timeout fallback", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Timeout: &metav1.Duration{Duration: time.Minute},
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Namespace).To(Equal(obj.Namespace))
		g.Expect(got.Timeout).To(Equal(obj.Spec.Timeout.Duration))
	})

	t.Run("applies options", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, []UpgradeOption{
			func(upgrade *helmaction.Upgrade) {
				upgrade.Install = true
			},
			func(upgrade *helmaction.Upgrade) {
				upgrade.DryRunStrategy = helmaction.DryRunClient
			},
		})
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Install).To(BeTrue())
		g.Expect(got.DryRunStrategy).To(Equal(helmaction.DryRunClient))
	})

	t.Run("disable take ownership", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Upgrade: &v2.Upgrade{
					DisableTakeOwnership: true,
				},
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.TakeOwnership).To(BeFalse())
	})

	t.Run("server side apply is auto regardless of UseHelm3Defaults", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		// Save and restore UseHelm3Defaults
		oldUseHelm3Defaults := UseHelm3Defaults
		t.Cleanup(func() { UseHelm3Defaults = oldUseHelm3Defaults })

		// Test with UseHelm3Defaults = false
		UseHelm3Defaults = false
		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("auto"))

		// Test with UseHelm3Defaults = true
		UseHelm3Defaults = true
		got = newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("auto"))
	})

	t.Run("server side apply user specified", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				Upgrade: &v2.Upgrade{
					ServerSideApply: v2.ServerSideApplyEnabled,
				},
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("true"))

		obj.Spec.Upgrade.ServerSideApply = v2.ServerSideApplyDisabled
		got = newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.ServerSideApply).To(Equal("false"))
	})

	t.Run("post render strategy defaults to combined with Helm4 defaults", func(t *testing.T) {
		g := NewWithT(t)

		// Save and restore UseHelm3Defaults
		oldUseHelm3Defaults := UseHelm3Defaults
		t.Cleanup(func() { UseHelm3Defaults = oldUseHelm3Defaults })
		UseHelm3Defaults = false

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.PostRenderStrategy).To(Equal(action.PostRenderStrategyCombined))
	})

	t.Run("post render strategy defaults to nohooks with UseHelm3Defaults", func(t *testing.T) {
		g := NewWithT(t)

		// Save and restore UseHelm3Defaults
		oldUseHelm3Defaults := UseHelm3Defaults
		t.Cleanup(func() { UseHelm3Defaults = oldUseHelm3Defaults })
		UseHelm3Defaults = true

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.PostRenderStrategy).To(Equal(action.PostRenderStrategyNoHooks))
	})

	t.Run("post render strategy combined", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				PostRenderStrategy: v2.PostRenderStrategyCombined,
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.PostRenderStrategy).To(Equal(action.PostRenderStrategyCombined))
	})

	t.Run("post render strategy separate", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				PostRenderStrategy: v2.PostRenderStrategySeparate,
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.PostRenderStrategy).To(Equal(action.PostRenderStrategySeparate))
	})

	t.Run("post render strategy nohooks", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "upgrade",
				Namespace: "upgrade-ns",
			},
			Spec: v2.HelmReleaseSpec{
				PostRenderStrategy: v2.PostRenderStrategyNoHooks,
			},
		}

		got := newUpgrade(&helmaction.Configuration{}, obj, nil)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.PostRenderStrategy).To(Equal(action.PostRenderStrategyNoHooks))
	})
}
