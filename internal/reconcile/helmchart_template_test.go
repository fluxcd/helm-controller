/*
Copyright 2023 The Flux authors

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

package reconcile

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"helm.sh/helm/v4/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/events"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/acl"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

func TestHelmChartTemplate_Reconcile(t *testing.T) {
	g := NewWithT(t)

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "helm-release-chart-reconciler-",
		},
	}
	g.Expect(testEnv.CreateAndWait(context.Background(), &namespace)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(testEnv.Cleanup(context.Background(), &namespace)).To(Succeed())
	})

	t.Run("DeletionTimestamp triggers delete", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "deletion-timestamp"
		existingChart := sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.HelmChartSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart:    "foo",
				SourceRef: sourcev1.LocalHelmChartSourceReference{
					Kind: sourcev1.HelmRepositoryKind,
					Name: "foo-repository",
				},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.Background(), &existingChart)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingChart)).To(Succeed())
		})

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		ts := metav1.Now()
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         namespace.GetName(),
				Name:              releaseName,
				DeletionTimestamp: &ts,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", existingChart.GetNamespace(), existingChart.GetName()),
			},
		}

		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		g.Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(testEnv.Get(context.TODO(),
				types.NamespacedName{
					Namespace: existingChart.GetNamespace(),
					Name:      existingChart.GetName(),
				},
				&existingChart,
			))).To(BeTrue())
		}).Should(Succeed())
	})

	t.Run("Status.HelmChart divergence triggers delete and creates chart", func(t *testing.T) {
		g := NewWithT(t)

		existingChart := sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace.GetName(),
				GenerateName: "existing-chart-",
			},
			Spec: sourcev1.HelmChartSpec{
				SourceRef: sourcev1.LocalHelmChartSourceReference{
					Kind: sourcev1.HelmRepositoryKind,
					Name: "mock",
				},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.TODO(), &existingChart)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingChart)).To(Succeed())
		})

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      "release-with-existing-chart",
			},
			Spec: v2.HelmReleaseSpec{
				Chart: &v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						Chart: "foo",
						SourceRef: &v2.CrossNamespaceObjectReference{
							Kind: sourcev1.HelmRepositoryKind,
							Name: "foo-repository",
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", existingChart.GetNamespace(), existingChart.GetName()),
			},
		}

		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(Equal(
			fmt.Sprintf("%s/%s", namespace.GetName(), namespace.GetName()+"-"+obj.GetName()),
		))

		g.Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(testEnv.Get(context.TODO(),
				types.NamespacedName{
					Namespace: existingChart.GetNamespace(),
					Name:      existingChart.GetName(),
				},
				&existingChart,
			))).To(BeTrue())
		}).Should(Succeed())
	})

	t.Run("HelmChart NotFound creates HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		releaseName := "not-found"
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						SourceRef: &v2.CrossNamespaceObjectReference{
							Kind: sourcev1.HelmRepositoryKind,
							Name: "mock",
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", namespace.GetName(), namespace.GetName()+"-"+releaseName),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		expectChart := sourcev1.HelmChart{}
		g.Eventually(func(g Gomega) {
			g.Expect(testEnv.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(),
				&expectChart,
			)).To(Succeed())
		}).Should(Succeed())

		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &expectChart)).To(Succeed())
		})
	})

	t.Run("Spec divergence updates HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "divergence"
		existingChart := sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.HelmChartSpec{
				Chart: "./bar",
				SourceRef: sourcev1.LocalHelmChartSourceReference{
					Kind: sourcev1.HelmRepositoryKind,
					Name: "bar-repository",
				},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.TODO(), &existingChart)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingChart)).To(Succeed())
		})

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						Chart: "foo",
						SourceRef: &v2.CrossNamespaceObjectReference{
							Kind: sourcev1.HelmRepositoryKind,
							Name: "foo-repository",
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", existingChart.GetNamespace(), existingChart.GetName()),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		newChart := sourcev1.HelmChart{}
		g.Eventually(func(g Gomega) {
			g.Expect(testEnv.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(), &newChart)).To(Succeed())

			g.Expect(newChart.Spec.Chart).To(Equal(obj.Spec.Chart.Spec.Chart))
			g.Expect(newChart.Spec.SourceRef.Name).To(Equal(obj.Spec.Chart.Spec.SourceRef.Name))
			g.Expect(newChart.Spec.SourceRef.Kind).To(Equal(obj.Spec.Chart.Spec.SourceRef.Kind))
		}).Should(Succeed())
	})

	t.Run("no HelmChart divergence", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "no-divergence"
		existingChart := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.HelmChartSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart:    "foo",
				SourceRef: sourcev1.LocalHelmChartSourceReference{
					Kind: sourcev1.HelmRepositoryKind,
					Name: "foo-repository",
				},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.Background(), existingChart)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), existingChart)).To(Succeed())
		})

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: existingChart.Spec.Interval,
				Chart: &v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						Chart: existingChart.Spec.Chart,
						SourceRef: &v2.CrossNamespaceObjectReference{
							Kind: existingChart.Spec.SourceRef.Kind,
							Name: existingChart.Spec.SourceRef.Name,
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", existingChart.GetNamespace(), existingChart.GetName()),
			},
		}

		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		newChart := sourcev1.HelmChart{}
		g.Expect(testEnv.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(), &newChart)).To(Succeed())
		g.Expect(newChart.ResourceVersion).To(Equal(existingChart.ResourceVersion), "HelmChart should not have been updated")
	})

	t.Run("sets owner labels on HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		releaseName := "owner-labels"
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						SourceRef: &v2.CrossNamespaceObjectReference{
							Kind: sourcev1.HelmRepositoryKind,
							Name: "mock",
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", namespace.GetName(), namespace.GetName()+"-"+releaseName),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		expectChart := sourcev1.HelmChart{}
		g.Eventually(func(g Gomega) {
			g.Expect(r.client.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(), &expectChart)).To(Succeed())
			g.Expect(testEnv.Cleanup(context.Background(), &expectChart)).To(Succeed())

			g.Expect(expectChart.GetLabels()).To(HaveKeyWithValue(v2.GroupVersion.Group+"/name", obj.GetName()))
			g.Expect(expectChart.GetLabels()).To(HaveKeyWithValue(v2.GroupVersion.Group+"/namespace", obj.GetNamespace()))
		}).Should(Succeed())
	})

	t.Run("cross namespace disallow is respected", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmChartTemplate{
			client: fake.NewClientBuilder().WithScheme(NewTestScheme()).Build(),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Spec: v2.HelmReleaseSpec{
				Chart: &v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						SourceRef: &v2.CrossNamespaceObjectReference{
							Name:      "chart",
							Namespace: "other",
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).To(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: "other", Name: "chart"}, &sourcev1.HelmChart{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("Spec ChartRef and existing chart trigger delete", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "garbage-collection"
		existingChart := sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.HelmChartSpec{
				Chart: "./bar",
				SourceRef: sourcev1.LocalHelmChartSourceReference{
					Kind: sourcev1.HelmRepositoryKind,
					Name: "bar-repository",
				},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.TODO(), &existingChart)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingChart)).To(Succeed())
		})

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: events.NewFakeRecorder(32, false),
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind: sourcev1.OCIRepositoryKind,
					Name: "oci-repository",
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", existingChart.GetNamespace(), existingChart.GetName()),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})
}

func TestHelmChartTemplate_reconcileDelete(t *testing.T) {
	now := metav1.Now()

	t.Run("Status.HelmChart is deleted", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(&sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			})

		r := &HelmChartTemplate{
			client:        builder.Build(),
			eventRecorder: events.NewFakeRecorder(32, false),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		srcObj := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "chart",
				Namespace: "default",
			},
		}
		err := r.reconcileDelete(context.TODO(), obj, srcObj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "chart"}, &sourcev1.HelmChart{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("Status.HelmChart is cleared when delete returns NotFound", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(&sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			}).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					return apierrors.NewNotFound(sourcev1.GroupVersion.WithResource("helmcharts").GroupResource(), obj.GetName())
				},
			})

		r := &HelmChartTemplate{
			client:        builder.Build(),
			eventRecorder: events.NewFakeRecorder(32, false),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		err := r.reconcileDelete(context.TODO(), obj, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})

	t.Run("Status.HelmChart already deleted", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmChartTemplate{
			client: fake.NewClientBuilder().WithScheme(NewTestScheme()).Build(),
		}
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		srcObj := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "chart",
				Namespace: "default",
			},
		}
		err := r.reconcileDelete(context.TODO(), obj, srcObj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})

	t.Run("Spec.Suspend is respected", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(&sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			})

		r := &HelmChartTemplate{
			client:        builder.Build(),
			eventRecorder: events.NewFakeRecorder(32, false),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "release",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		srcObj := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "chart",
				Namespace: "default",
			},
		}
		err := r.reconcileDelete(context.TODO(), obj, srcObj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		g.Consistently(func(g Gomega) {
			err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "chart"}, &sourcev1.HelmChart{})
			g.Expect(err).ToNot(HaveOccurred())
		}).Should(Succeed())
	})

	t.Run("cross namespace allow is respected", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other",
				Name:      "chart",
			},
		}
		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(chart)

		r := &HelmChartTemplate{
			client: builder.Build(),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "other/chart",
			},
		}
		srcObj := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "chart",
				Namespace: "default",
			},
		}

		currentAllow := acl.AllowCrossNamespaceRef
		acl.AllowCrossNamespaceRef = false
		t.Cleanup(func() { acl.AllowCrossNamespaceRef = currentAllow })

		err := r.reconcileDelete(context.TODO(), obj, srcObj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		g.Expect(r.client.Get(context.TODO(),
			types.NamespacedName{Namespace: chart.Namespace, Name: chart.Name},
			&sourcev1.HelmChart{}),
		).To(Succeed())
	})

	t.Run("empty Status.HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmChartTemplate{
			client: fake.NewClientBuilder().WithScheme(NewTestScheme()).Build(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{},
		}
		srcObj := &sourcev1.HelmChart{
			Status: sourcev1.HelmChartStatus{},
		}
		err := r.reconcileDelete(context.TODO(), obj, srcObj)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func Test_buildHelmChartFromTemplate(t *testing.T) {
	hrWithChartTemplate := v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-release",
			Namespace: "default",
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: time.Minute},
			Chart: &v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: &v2.CrossNamespaceObjectReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						Interval: &metav1.Duration{Duration: 2 * time.Minute},
					},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
	}

	tests := []struct {
		name   string
		modify func(release *v2.HelmRelease)
		want   *sourcev1.HelmChart
	}{
		{
			name:   "builds HelmChart from HelmChartTemplate",
			modify: func(*v2.HelmRelease) {},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyChartVersion,
					Interval:          metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles:       []string{"values.yaml"},
				},
			},
		},
		{
			name: "defaults version to latest",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Version = ""
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "*",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyChartVersion,
					Interval:          metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles:       []string{"values.yaml"},
				},
			},
		},
		{
			name: "takes ReconcileStrategy into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.ReconcileStrategy = sourcev1.ReconcileStrategyRevision
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyRevision,
					Interval:          metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles:       []string{"values.yaml"},
				},
			},
		},
		{
			name: "takes SourceRef namespace into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.SourceRef.Namespace = "cross"
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "cross",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyChartVersion,
					Interval:          metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles:       []string{"values.yaml"},
				},
			},
		},
		{
			name: "falls back to HelmRelease interval",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Interval = nil
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyChartVersion,
					Interval:          metav1.Duration{Duration: time.Minute},
					ValuesFiles:       []string{"values.yaml"},
				},
			},
		},
		{
			name: "take cosign verification into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Verify = &sourcev1.OCIRepositoryVerification{
					Provider: "cosign",
					SecretRef: &meta.LocalObjectReference{
						Name: "cosign-key",
					},
				}
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyChartVersion,
					Interval:          metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles:       []string{"values.yaml"},
					Verify: &sourcev1.HelmChartVerification{
						Provider: "cosign",
						SecretRef: &meta.LocalObjectReference{
							Name: "cosign-key",
						},
					},
				},
			},
		},
		{
			name: "takes object meta into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.ObjectMeta = &v2.HelmChartTemplateObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				}
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyChartVersion,
					Interval:          metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles:       []string{"values.yaml"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			hr := hrWithChartTemplate.DeepCopy()
			tt.modify(hr)

			g.Expect(buildHelmChartFromTemplate(hr, hr.GetHelmChartTemplateReference())).To(Equal(tt.want))
		})
	}
}

func Test_buildOCIRepositoryFromTemplate(t *testing.T) {
	defaultLayerSelector := &sourcev1.OCILayerSelector{
		MediaType: registry.ChartLayerMediaType,
		Operation: sourcev1.OCILayerCopy,
	}

	hrWithChartTemplate := v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-release",
			Namespace: "default",
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: time.Minute},
			Chart: &v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
						Reference: &sourcev1.OCIRepositoryRef{
							SemVer: ">= 6.0.0",
						},
						Interval: &metav1.Duration{Duration: 2 * time.Minute},
					},
				},
			},
		},
	}

	tests := []struct {
		name   string
		modify func(release *v2.HelmRelease)
		want   *sourcev1.OCIRepository
	}{
		{
			name:   "builds OCIRepository from HelmChartTemplate",
			modify: func(*v2.HelmRelease) {},
			want: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.OCIRepositorySpec{
					URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					Reference: &sourcev1.OCIRepositoryRef{
						SemVer: ">= 6.0.0",
					},
					LayerSelector: defaultLayerSelector,
					Interval:      &metav1.Duration{Duration: 2 * time.Minute},
				},
			},
		},
		{
			name: "falls back to HelmRelease interval",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Interval = nil
			},
			want: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.OCIRepositorySpec{
					URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					Reference: &sourcev1.OCIRepositoryRef{
						SemVer: ">= 6.0.0",
					},
					LayerSelector: defaultLayerSelector,
					Interval:      &metav1.Duration{Duration: time.Minute},
				},
			},
		},
		{
			name: "respects explicitly specified layer selector",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.LayerSelector = &sourcev1.OCILayerSelector{
					MediaType: "application/vnd.example.custom.layer",
					Operation: sourcev1.OCILayerExtract,
				}
			},
			want: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.OCIRepositorySpec{
					URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					Reference: &sourcev1.OCIRepositoryRef{
						SemVer: ">= 6.0.0",
					},
					LayerSelector: &sourcev1.OCILayerSelector{
						MediaType: "application/vnd.example.custom.layer",
						Operation: sourcev1.OCILayerExtract,
					},
					Interval: &metav1.Duration{Duration: 2 * time.Minute},
				},
			},
		},
		{
			name: "take cosign verification into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Verify = &sourcev1.OCIRepositoryVerification{
					Provider: "cosign",
					SecretRef: &meta.LocalObjectReference{
						Name: "cosign-key",
					},
				}
			},
			want: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.OCIRepositorySpec{
					URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					Reference: &sourcev1.OCIRepositoryRef{
						SemVer: ">= 6.0.0",
					},
					LayerSelector: defaultLayerSelector,
					Interval:      &metav1.Duration{Duration: 2 * time.Minute},
					Verify: &sourcev1.OCIRepositoryVerification{
						Provider: "cosign",
						SecretRef: &meta.LocalObjectReference{
							Name: "cosign-key",
						},
					},
				},
			},
		},
		{
			name: "takes object meta into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.ObjectMeta = &v2.HelmChartTemplateObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				}
			},
			want: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				},
				Spec: sourcev1.OCIRepositorySpec{
					URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					Reference: &sourcev1.OCIRepositoryRef{
						SemVer: ">= 6.0.0",
					},
					LayerSelector: defaultLayerSelector,
					Interval:      &metav1.Duration{Duration: 2 * time.Minute},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			hr := hrWithChartTemplate.DeepCopy()
			tt.modify(hr)

			g.Expect(buildOCIRepositoryFromTemplate(hr, hr.GetHelmChartTemplateReference())).To(Equal(tt.want))
		})
	}
}

func TestHelmChartTemplate_reconcileDelete_OCIRepository(t *testing.T) {
	now := metav1.Now()

	t.Run("Status.OCIRepository is deleted", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(&sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			})

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        builder.Build(),
			eventRecorder: recorder,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/default/chart", sourcev1.OCIRepositoryKind),
			},
		}
		err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "chart"}, &sourcev1.OCIRepository{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("Status.OCIRepository is cleared when delete returns NotFound", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(&sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			}).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					return apierrors.NewNotFound(sourcev1.GroupVersion.WithResource("ocirepositories").GroupResource(), obj.GetName())
				},
			})

		r := &HelmChartTemplate{
			client:        builder.Build(),
			eventRecorder: record.NewFakeRecorder(32),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/default/chart", sourcev1.OCIRepositoryKind),
			},
		}
		err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})

	t.Run("Status.OCIRepository already deleted", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmChartTemplate{
			client: fake.NewClientBuilder().WithScheme(NewTestScheme()).Build(),
		}
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/default/chart", sourcev1.OCIRepositoryKind),
			},
		}
		err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})

	t.Run("Spec.Suspend is respected", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(&sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			})

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        builder.Build(),
			eventRecorder: recorder,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "release",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/default/chart", sourcev1.OCIRepositoryKind),
			},
		}
		err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		g.Consistently(func(g Gomega) {
			err = r.client.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "chart"}, &sourcev1.OCIRepository{})
			g.Expect(err).ToNot(HaveOccurred())
		}).Should(Succeed())
	})

	t.Run("cross namespace allow is respected", func(t *testing.T) {
		g := NewWithT(t)

		repo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other",
				Name:      "chart",
			},
		}
		builder := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithObjects(repo)

		r := &HelmChartTemplate{
			client: builder.Build(),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/other/chart", sourcev1.OCIRepositoryKind),
			},
		}

		currentAllow := acl.AllowCrossNamespaceRef
		acl.AllowCrossNamespaceRef = false
		t.Cleanup(func() { acl.AllowCrossNamespaceRef = currentAllow })

		err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		g.Expect(r.client.Get(context.TODO(),
			types.NamespacedName{Namespace: repo.Namespace, Name: repo.Name},
			&sourcev1.OCIRepository{}),
		).To(Succeed())
	})
}

func TestHelmChartTemplate_Reconcile_OCIRepository(t *testing.T) {
	g := NewWithT(t)

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "helm-release-chart-reconciler-",
		},
	}
	g.Expect(testEnv.CreateAndWait(context.Background(), &namespace)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(testEnv.Cleanup(context.Background(), &namespace)).To(Succeed())
	})

	t.Run("DeletionTimestamp triggers delete", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "oci-deletion-timestamp"
		existingRepo := sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL:      "oci://ghcr.io/stefanprodan/charts/podinfo",
				Interval: &metav1.Duration{Duration: 1 * time.Hour},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.Background(), &existingRepo)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingRepo)).To(Succeed())
		})

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: recorder,
			fieldManager:  testFieldManager,
		}

		ts := metav1.Now()
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         namespace.GetName(),
				Name:              releaseName,
				DeletionTimestamp: &ts,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, existingRepo.GetNamespace(), existingRepo.GetName()),
			},
		}

		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		g.Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(testEnv.Get(context.TODO(),
				types.NamespacedName{
					Namespace: existingRepo.GetNamespace(),
					Name:      existingRepo.GetName(),
				},
				&existingRepo,
			))).To(BeTrue())
		}).Should(Succeed())
	})

	t.Run("Status.OCIRepository divergence triggers delete and creates chart", func(t *testing.T) {
		g := NewWithT(t)

		existingRepo := sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace.GetName(),
				GenerateName: "existing-oci-repo-",
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL:      "oci://ghcr.io/stefanprodan/charts/podinfo",
				Interval: &metav1.Duration{Duration: 1 * time.Hour},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.TODO(), &existingRepo)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingRepo)).To(Succeed())
		})

		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: record.NewFakeRecorder(32),
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      "oci-release-with-existing-repo",
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Kind: sourcev1.OCIRepositoryKind,
					Spec: v2.HelmChartTemplateSpec{
						OCIRepositorySpec: sourcev1.OCIRepositorySpec{
							URL:      "oci://ghcr.io/stefanprodan/charts/podinfo",
							Interval: &metav1.Duration{Duration: 1 * time.Hour},
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, existingRepo.GetNamespace(), existingRepo.GetName()),
			},
		}

		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(Equal(
			fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, namespace.GetName(), namespace.GetName()+"-"+obj.GetName()),
		))

		g.Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(testEnv.Get(context.TODO(),
				types.NamespacedName{
					Namespace: existingRepo.GetNamespace(),
					Name:      existingRepo.GetName(),
				},
				&existingRepo,
			))).To(BeTrue())
		}).Should(Succeed())
	})

	t.Run("OCIRepository NotFound creates OCIRepository", func(t *testing.T) {
		g := NewWithT(t)

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: recorder,
			fieldManager:  testFieldManager,
		}

		releaseName := "oci-not-found"
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Kind: sourcev1.OCIRepositoryKind,
					Spec: v2.HelmChartTemplateSpec{
						OCIRepositorySpec: sourcev1.OCIRepositorySpec{
							URL:      "oci://ghcr.io/stefanprodan/charts/podinfo",
							Interval: &metav1.Duration{Duration: 1 * time.Hour},
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, namespace.GetName(), namespace.GetName()+"-"+releaseName),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		expectRepo := sourcev1.OCIRepository{}
		g.Eventually(func(g Gomega) {
			g.Expect(testEnv.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(),
				&expectRepo,
			)).To(Succeed())
		}).Should(Succeed())

		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &expectRepo)).To(Succeed())
		})
	})

	t.Run("Spec divergence updates OCIRepository", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "oci-divergence"
		existingRepo := sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL:      "oci://ghcr.io/stefanprodan/charts/bar",
				Interval: &metav1.Duration{Duration: 1 * time.Hour},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.TODO(), &existingRepo)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingRepo)).To(Succeed())
		})

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: recorder,
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Kind: sourcev1.OCIRepositoryKind,
					Spec: v2.HelmChartTemplateSpec{
						OCIRepositorySpec: sourcev1.OCIRepositorySpec{
							URL:      "oci://ghcr.io/stefanprodan/charts/foo",
							Interval: &metav1.Duration{Duration: 1 * time.Hour},
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, existingRepo.GetNamespace(), existingRepo.GetName()),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		newRepo := sourcev1.OCIRepository{}
		g.Eventually(func(g Gomega) {
			g.Expect(testEnv.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(), &newRepo)).To(Succeed())

			g.Expect(newRepo.Spec.URL).To(Equal(obj.Spec.Chart.Spec.OCIRepositorySpec.URL))
		}).Should(Succeed())
	})

	t.Run("no OCIRepository divergence", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "oci-no-divergence"
		existingRepo := &sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
				LayerSelector: &sourcev1.OCILayerSelector{
					MediaType: registry.ChartLayerMediaType,
					Operation: sourcev1.OCILayerCopy,
				},
				Interval: &metav1.Duration{Duration: 1 * time.Hour},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.Background(), existingRepo)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), existingRepo)).To(Succeed())
		})

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: recorder,
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: *existingRepo.Spec.Interval,
				Chart: &v2.HelmChartTemplate{
					Kind: sourcev1.OCIRepositoryKind,
					Spec: v2.HelmChartTemplateSpec{
						OCIRepositorySpec: sourcev1.OCIRepositorySpec{
							URL:      existingRepo.Spec.URL,
							Interval: existingRepo.Spec.Interval,
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, existingRepo.GetNamespace(), existingRepo.GetName()),
			},
		}

		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		newRepo := sourcev1.OCIRepository{}
		g.Expect(testEnv.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(), &newRepo)).To(Succeed())
		g.Expect(newRepo.ResourceVersion).To(Equal(existingRepo.ResourceVersion), "OCIRepository should not have been updated")
	})

	t.Run("sets owner labels on OCIRepository", func(t *testing.T) {
		g := NewWithT(t)

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: recorder,
			fieldManager:  testFieldManager,
		}

		releaseName := "oci-owner-labels"
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: &v2.HelmChartTemplate{
					Kind: sourcev1.OCIRepositoryKind,
					Spec: v2.HelmChartTemplateSpec{
						OCIRepositorySpec: sourcev1.OCIRepositorySpec{
							URL:      "oci://ghcr.io/stefanprodan/charts/podinfo",
							Interval: &metav1.Duration{Duration: 1 * time.Hour},
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, namespace.GetName(), namespace.GetName()+"-"+releaseName),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		expectRepo := sourcev1.OCIRepository{}
		g.Eventually(func(g Gomega) {
			g.Expect(r.client.Get(context.TODO(), obj.GetHelmChartTemplateReference().GetObjectKey(), &expectRepo)).To(Succeed())
			g.Expect(testEnv.Cleanup(context.Background(), &expectRepo)).To(Succeed())

			g.Expect(expectRepo.GetLabels()).To(HaveKeyWithValue(v2.GroupVersion.Group+"/name", obj.GetName()))
			g.Expect(expectRepo.GetLabels()).To(HaveKeyWithValue(v2.GroupVersion.Group+"/namespace", obj.GetNamespace()))
		}).Should(Succeed())
	})

	t.Run("Spec ChartRef and existing repo trigger delete", func(t *testing.T) {
		g := NewWithT(t)

		releaseName := "oci-garbage-collection"
		existingRepo := sourcev1.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      fmt.Sprintf("%s-%s", namespace.GetName(), releaseName),
				Labels: map[string]string{
					v2.GroupVersion.Group + "/name":      releaseName,
					v2.GroupVersion.Group + "/namespace": namespace.GetName(),
				},
			},
			Spec: sourcev1.OCIRepositorySpec{
				URL:      "oci://ghcr.io/stefanprodan/charts/bar",
				Interval: &metav1.Duration{Duration: 1 * time.Hour},
			},
		}
		g.Expect(testEnv.CreateAndWait(context.TODO(), &existingRepo)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &existingRepo)).To(Succeed())
		})

		recorder := record.NewFakeRecorder(32)
		r := &HelmChartTemplate{
			client:        testEnv,
			eventRecorder: recorder,
			fieldManager:  testFieldManager,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind: sourcev1.OCIRepositoryKind,
					Name: "oci-repository",
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s/%s", sourcev1.OCIRepositoryKind, existingRepo.GetNamespace(), existingRepo.GetName()),
			},
		}
		err := r.Reconcile(context.TODO(), &Request{Object: obj})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})
}
