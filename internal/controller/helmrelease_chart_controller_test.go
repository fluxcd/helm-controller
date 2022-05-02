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

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestHelmReleaseChartReconciler_Reconcile(t *testing.T) {
	t.Run("reconciles HelmChartTemplate", func(t *testing.T) {
		g := NewWithT(t)

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "helm-release-chart-",
			},
		}
		g.Expect(testEnv.CreateAndWait(context.Background(), namespace)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), namespace)).To(Succeed())
		})

		chartSpecTemplate := v2.HelmChartTemplateSpec{
			Chart: "chart",
			SourceRef: v2.CrossNamespaceObjectReference{
				Kind: sourcev1.HelmRepositoryKind,
				Name: "repository",
			},
		}
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      "reconcile",
				Finalizers: []string{
					v2.ChartFinalizer,
				},
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Millisecond},
				Chart: v2.HelmChartTemplate{
					Spec: chartSpecTemplate,
				},
			},
		}

		g.Expect(testEnv.CreateAndWait(context.TODO(), obj.DeepCopy())).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.TODO(), obj)).To(Succeed())
		})

		r := &HelmReleaseChartReconciler{
			Client:        testEnv,
			EventRecorder: record.NewFakeRecorder(32),
			FieldManager:  "helm-controller",
		}

		key := types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		got, err := r.Reconcile(ctrl.LoggerInto(context.TODO(), logr.Discard()), reconcile.Request{NamespacedName: key})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{RequeueAfter: obj.Spec.Interval.Duration}))

		g.Expect(testClient.Get(context.TODO(), key, obj)).To(Succeed())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		chartNs, chartName := obj.Status.GetHelmChart()
		var chartObj sourcev1.HelmChart
		g.Expect(r.Client.Get(context.TODO(), types.NamespacedName{Namespace: chartNs, Name: chartName}, &chartObj)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(testEnv.Cleanup(context.Background(), &chartObj)).To(Succeed())
		})

		g.Expect(chartObj.Spec.Chart).To(Equal(obj.Spec.Chart.Spec.Chart))
	})

	t.Run("HelmRelease NotFound", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(testScheme)
		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		key := types.NamespacedName{
			Name:      "not",
			Namespace: "found",
		}
		got, err := r.Reconcile(ctrl.LoggerInto(context.TODO(), logr.Discard()), reconcile.Request{NamespacedName: key})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))
	})

	t.Run("finalizer set before start reconciliation", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
		}

		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(obj)
		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		key := types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		got, err := r.Reconcile(ctrl.LoggerInto(context.TODO(), logr.Discard()), reconcile.Request{NamespacedName: key})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{Requeue: true}))

		g.Expect(r.Client.Get(context.TODO(), key, obj)).To(Succeed())
		g.Expect(controllerutil.ContainsFinalizer(obj, v2.ChartFinalizer)).To(BeTrue())
	})

	t.Run("HelmRelease suspended", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
				Finalizers: []string{
					v2.ChartFinalizer,
				},
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
		}

		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(obj)
		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		key := types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		got, err := r.Reconcile(ctrl.LoggerInto(context.TODO(), logr.Discard()), reconcile.Request{NamespacedName: key})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))
	})

	t.Run("DeletionTimestamp triggers delete", func(t *testing.T) {
		g := NewWithT(t)

		now := metav1.Now()
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "release",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers: []string{
					v2.ChartFinalizer,
					sourcev1.SourceFinalizer,
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/does-not-exist",
			},
		}

		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(obj).
			WithStatusSubresource(&v2.HelmRelease{})

		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		key := types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		got, err := r.Reconcile(ctrl.LoggerInto(context.TODO(), logr.Discard()), reconcile.Request{NamespacedName: key})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))

		g.Expect(r.Client.Get(context.TODO(), key, obj)).To(Succeed())
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
		g.Expect(controllerutil.ContainsFinalizer(obj, v2.ChartFinalizer)).To(BeFalse())
	})

	t.Run("DeletionTimestamp with Suspend removes finalizer", func(t *testing.T) {
		g := NewWithT(t)

		now := metav1.Now()
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "release",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers: []string{
					v2.ChartFinalizer,
					sourcev1.SourceFinalizer,
				},
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/does-not-exist",
			},
		}

		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(obj).
			WithStatusSubresource(&v2.HelmRelease{})

		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		key := types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		got, err := r.Reconcile(ctrl.LoggerInto(context.TODO(), logr.Discard()), reconcile.Request{NamespacedName: key})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))

		g.Expect(r.Client.Get(context.TODO(), key, obj)).To(Succeed())
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())
		g.Expect(controllerutil.ContainsFinalizer(obj, v2.ChartFinalizer)).To(BeFalse())
	})
}

func TestHelmReleaseChartReconciler_reconcile(t *testing.T) {
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

	t.Run("Status.HelmChart divergence triggers delete and requeue", func(t *testing.T) {
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

		r := &HelmReleaseChartReconciler{
			Client:        testEnv,
			EventRecorder: record.NewFakeRecorder(32),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      "release-with-existing-chart",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: fmt.Sprintf("%s/%s", existingChart.GetNamespace(), existingChart.GetName()),
			},
		}
		got, err := r.reconcile(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
	})

	t.Run("HelmChart NotFound creates HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		recorder := record.NewFakeRecorder(32)
		r := &HelmReleaseChartReconciler{
			Client:        testEnv,
			EventRecorder: recorder,
			FieldManager:  "helm-controller",
		}

		releaseName := "not-found"
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						SourceRef: v2.CrossNamespaceObjectReference{
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
		got, err := r.reconcile(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}))
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		expectChart := sourcev1.HelmChart{}
		g.Expect(testClient.Get(context.TODO(), types.NamespacedName{
			Namespace: obj.Spec.Chart.GetNamespace(obj.Namespace),
			Name:      obj.GetHelmChartName()},
			&expectChart,
		)).To(Succeed())
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

		recorder := record.NewFakeRecorder(32)
		r := &HelmReleaseChartReconciler{
			Client:        testEnv,
			EventRecorder: recorder,
			FieldManager:  "helm-controller",
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						Chart: "foo",
						SourceRef: v2.CrossNamespaceObjectReference{
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
		got, err := r.reconcile(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}))
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		newChart := sourcev1.HelmChart{}
		g.Expect(testClient.Get(context.TODO(), types.NamespacedName{
			Namespace: obj.Spec.Chart.GetNamespace(obj.Namespace),
			Name:      obj.GetHelmChartName()}, &newChart)).To(Succeed())

		g.Expect(newChart.Spec.Chart).To(Equal(obj.Spec.Chart.Spec.Chart))
		g.Expect(newChart.Spec.SourceRef.Name).To(Equal(obj.Spec.Chart.Spec.SourceRef.Name))
		g.Expect(newChart.Spec.SourceRef.Kind).To(Equal(obj.Spec.Chart.Spec.SourceRef.Kind))
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

		recorder := record.NewFakeRecorder(32)
		r := &HelmReleaseChartReconciler{
			Client:        testEnv,
			EventRecorder: recorder,
			FieldManager:  "helm-controller",
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: existingChart.Spec.Interval,
				Chart: v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						Chart: existingChart.Spec.Chart,
						SourceRef: v2.CrossNamespaceObjectReference{
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

		got, err := r.reconcile(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}))
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		newChart := sourcev1.HelmChart{}
		g.Expect(testClient.Get(context.TODO(), types.NamespacedName{
			Namespace: obj.Spec.Chart.GetNamespace(obj.Namespace),
			Name:      obj.GetHelmChartName()}, &newChart)).To(Succeed())
		g.Expect(newChart.ResourceVersion).To(Equal(existingChart.ResourceVersion), "HelmChart should not have been updated")
	})

	t.Run("sets owner labels on HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		recorder := record.NewFakeRecorder(32)
		r := &HelmReleaseChartReconciler{
			Client:        testEnv,
			EventRecorder: recorder,
			FieldManager:  "helm-controller",
		}

		releaseName := "owner-labels"
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.GetName(),
				Name:      releaseName,
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Hour},
				Chart: v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						SourceRef: v2.CrossNamespaceObjectReference{
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
		got, err := r.reconcile(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}))
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())

		expectChart := sourcev1.HelmChart{}
		g.Expect(r.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: obj.Spec.Chart.GetNamespace(obj.Namespace),
			Name:      obj.GetHelmChartName()},
			&expectChart,
		)).To(Succeed())
		g.Expect(testEnv.Cleanup(context.Background(), &expectChart)).To(Succeed())

		g.Expect(expectChart.GetLabels()).To(HaveKeyWithValue(v2.GroupVersion.Group+"/name", obj.GetName()))
		g.Expect(expectChart.GetLabels()).To(HaveKeyWithValue(v2.GroupVersion.Group+"/namespace", obj.GetNamespace()))
	})

	t.Run("cross namespace disallow is respected", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseChartReconciler{
			Client:              fake.NewClientBuilder().WithScheme(testScheme).Build(),
			NoCrossNamespaceRef: true,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "default",
			},
			Spec: v2.HelmReleaseSpec{
				Chart: v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						SourceRef: v2.CrossNamespaceObjectReference{
							Name:      "chart",
							Namespace: "other",
						},
					},
				},
			},
			Status: v2.HelmReleaseStatus{},
		}
		got, err := r.reconcile(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: "other", Name: "chart"}, &sourcev1.HelmChart{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}

func TestHelmReleaseChartReconciler_reconcileDelete(t *testing.T) {
	now := metav1.Now()

	t.Run("Status.HelmChart is deleted", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(&sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			})

		recorder := record.NewFakeRecorder(32)
		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: recorder,
		}

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		got, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(obj.Status.HelmChart).To(BeEmpty())

		err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "chart"}, &sourcev1.HelmChart{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("Status.HelmChart already deleted", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseChartReconciler{
			Client: fake.NewClientBuilder().WithScheme(testScheme).Build(),
		}
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: []string{v2.ChartFinalizer},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		got, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
		g.Expect(obj.Finalizers).To(ContainElement(v2.ChartFinalizer))
	})

	t.Run("DeletionTimestamp removes finalizer", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseChartReconciler{
			Client: fake.NewClientBuilder().WithScheme(testScheme).Build(),
		}
		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &now,
				Finalizers:        []string{v2.ChartFinalizer},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		got, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))
		g.Expect(obj.Status.HelmChart).To(BeEmpty())
		g.Expect(obj.Finalizers).ToNot(ContainElement(v2.ChartFinalizer))
	})

	t.Run("Spec.Suspend is respected", func(t *testing.T) {
		g := NewWithT(t)

		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(&sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "chart",
				},
			})

		recorder := record.NewFakeRecorder(32)
		r := &HelmReleaseChartReconciler{
			Client:        builder.Build(),
			EventRecorder: recorder,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &now,
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "default/chart",
			},
		}
		got, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())
		g.Expect(obj.Finalizers).ToNot(ContainElement(v2.ChartFinalizer))

		err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "chart"}, &sourcev1.HelmChart{})
		g.Expect(err).ToNot(HaveOccurred())

	})

	t.Run("cross namespace disallow is respected", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other",
				Name:      "chart",
			},
		}
		builder := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(chart)

		r := &HelmReleaseChartReconciler{
			Client:              builder.Build(),
			NoCrossNamespaceRef: true,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "other/chart",
			},
		}
		got, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{}))
		g.Expect(obj.Status.HelmChart).ToNot(BeEmpty())
		g.Expect(r.Client.Get(context.TODO(), types.NamespacedName{Namespace: chart.Namespace, Name: chart.Name}, &sourcev1.HelmChart{})).To(Succeed())
	})

	t.Run("empty Status.HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseChartReconciler{
			Client: fake.NewClientBuilder().WithScheme(testScheme).Build(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{},
		}
		got, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(ctrl.Result{Requeue: true}))
	})
}

func TestHelmReleaseChartReconciler_aclAllowAccessTo(t *testing.T) {
	tests := []struct {
		name           string
		obj            *v2.HelmRelease
		namespacedName types.NamespacedName
		allowCrossNS   bool
		wantErr        bool
	}{
		{
			name: "disallow cross namespace",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "a",
				},
			},
			namespacedName: types.NamespacedName{
				Namespace: "b",
				Name:      "foo",
			},
			allowCrossNS: false,
			wantErr:      true,
		},
		{
			name: "allow cross namespace",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "a",
				},
			},
			namespacedName: types.NamespacedName{
				Namespace: "b",
				Name:      "foo",
			},
			allowCrossNS: true,
		},
		{
			name: "same namespace disallow cross namespace",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "a",
				},
			},
			namespacedName: types.NamespacedName{
				Namespace: "a",
				Name:      "foo",
			},
			allowCrossNS: false,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &HelmReleaseChartReconciler{
				NoCrossNamespaceRef: !tt.allowCrossNS,
			}
			err := r.aclAllowAccessTo(tt.obj, tt.namespacedName)
			g.Expect(err != nil).To(Equal(tt.wantErr), err)
		})
	}
}

func Test_buildHelmChartFromTemplate(t *testing.T) {
	hrWithChartTemplate := v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-release",
			Namespace: "default",
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: time.Minute},
			Chart: v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: v2.CrossNamespaceObjectReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					Interval:    &metav1.Duration{Duration: 2 * time.Minute},
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
					Interval:    metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
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
					Interval:    metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
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
					Interval:    metav1.Duration{Duration: time.Minute},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
		{
			name: "take cosign verification into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Verify = &v2.HelmChartTemplateVerification{
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
					Interval:    metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
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
					Interval:    metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			hr := hrWithChartTemplate.DeepCopy()
			tt.modify(hr)

			g.Expect(buildHelmChartFromTemplate(hr)).To(Equal(tt.want))
		})
	}
}
