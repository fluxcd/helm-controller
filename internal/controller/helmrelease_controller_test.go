/*
Copyright 2020 The Flux authors

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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/fluxcd/pkg/apis/acl"
	aclv1 "github.com/fluxcd/pkg/apis/acl"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	feathelper "github.com/fluxcd/pkg/runtime/features"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	intacl "github.com/fluxcd/helm-controller/internal/acl"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/features"
	"github.com/fluxcd/helm-controller/internal/kube"
	intreconcile "github.com/fluxcd/helm-controller/internal/reconcile"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestHelmReleaseReconciler_reconcileRelease(t *testing.T) {
	t.Run("confirms dependencies are ready", func(t *testing.T) {
		g := NewWithT(t)

		dependency := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dependency",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:   meta.StalledCondition,
						Status: metav1.ConditionTrue,
					},
				},
				ObservedGeneration: 1,
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dependant",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				DependsOn: []meta.NamespacedObjectReference{
					{
						Name: "dependency",
					},
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(dependency, obj).
				Build(),
			EventRecorder:     record.NewFakeRecorder(32),
			requeueDependency: 5 * time.Second,
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForDependency))
		g.Expect(res.RequeueAfter).To(Equal(r.requeueDependency))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, meta.DependencyNotReadyReason, "dependency 'mock/dependency' is not ready"),
		}))
	})

	t.Run("handles HelmChart get failure", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "mock/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(obj).
				Build(),
		}

		_, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, "Fulfilling prerequisites"),
			*conditions.FalseCondition(meta.ReadyCondition, v2.ArtifactFailedReason, "could not get Source object"),
		}))
	})

	t.Run("handles ACL error for HelmChart", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "other/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(obj).
				Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, reconcile.TerminalError(nil))).To(BeTrue())
		g.Expect(res.IsZero()).To(BeTrue())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.StalledCondition, acl.AccessDeniedReason, "cross-namespace references are not allowed"),
			*conditions.FalseCondition(meta.ReadyCondition, acl.AccessDeniedReason, "cross-namespace references are not allowed"),
		}))
	})

	t.Run("waits for HelmChart to have an Artifact", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 2,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 2,
				Artifact:           nil,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "mock/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(chart, obj).
				Build(),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForChart))
		g.Expect(res.RequeueAfter).To(Equal(obj.Spec.Interval.Duration))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, "SourceNotReady", "HelmChart 'mock/chart' is not ready"),
		}))
	})

	t.Run("waits for HelmChart ObservedGeneration to equal Generation", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 2,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           &sourcev1.Artifact{},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "mock/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(chart, obj).
				Build(),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForChart))
		g.Expect(res.RequeueAfter).To(Equal(obj.Spec.Interval.Duration))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, "SourceNotReady", "HelmChart 'mock/chart' is not ready"),
		}))
	})

	t.Run("reports values composition failure", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 2,
			},
			Spec: sourcev1b2.HelmChartSpec{
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 2,
				Artifact:           &sourcev1.Artifact{},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ValuesFrom: []v2.ValuesReference{
					{
						Kind: "Secret",
						Name: "missing",
					},
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "mock/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(chart, obj).
				Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		_, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, "Fulfilling prerequisites"),
			*conditions.FalseCondition(meta.ReadyCondition, "ValuesError", "could not resolve Secret chart values reference 'mock/missing' with key 'values.yaml'"),
		}))
	})

	t.Run("reports Helm chart load failure", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact: &sourcev1.Artifact{
					URL: testServer.URL() + "/does-not-exist",
				},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "mock/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(chart, obj).
				Build(),
			requeueDependency: 10 * time.Second,
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForDependency))
		g.Expect(res.RequeueAfter).To(Equal(r.requeueDependency))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, v2.ArtifactFailedReason, "Source not ready"),
		}))
	})

	t.Run("attempts to adopt v2beta1 release state", func(t *testing.T) {
		g := NewWithT(t)

		// Initialize feature gates.
		g.Expect((&feathelper.FeatureGates{}).SupportedFeatures(features.FeatureGates())).To(Succeed())

		// Create a test namespace for storing the Helm release mock.
		ns, err := testEnv.CreateNamespace(context.TODO(), "adopt-release")
		g.Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), ns)
		})

		// Create HelmChart mock.
		chartMock := testutil.BuildChart()
		chartArtifact, err := testutil.SaveChartAsArtifact(chartMock, digest.SHA256, testServer.URL(), testServer.Root())
		g.Expect(err).ToNot(HaveOccurred())

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "adopt-release",
				Namespace:  ns.Name,
				Generation: 1,
			},
			Spec: sourcev1b2.HelmChartSpec{
				Chart:   "testdata/test-helmrepo",
				Version: "0.1.0",
				SourceRef: sourcev1b2.LocalHelmChartSourceReference{
					Kind: sourcev1b2.HelmRepositoryKind,
					Name: "reconcile-delete",
				},
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           chartArtifact,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		// Create a test Helm release storage mock.
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      "adopt-release",
			Namespace: ns.Name,
			Version:   1,
			Chart:     chartMock,
			Status:    helmrelease.StatusDeployed,
		}, testutil.ReleaseWithConfig(nil))
		valChecksum := chartutil.DigestValues("sha1", rls.Config)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "adopt-release",
				Namespace: ns.Name,
			},
			Spec: v2.HelmReleaseSpec{
				StorageNamespace: ns.Name,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart:                   chart.Namespace + "/" + chart.Name,
				LastReleaseRevision:         rls.Version,
				LastAttemptedValuesChecksum: valChecksum.Encoded(),
			},
		}

		c := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithStatusSubresource(&v2.HelmRelease{}).
			WithObjects(chart, obj).
			Build()

		r := &HelmReleaseReconciler{
			Client:           c,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		// Store the Helm release mock in the test namespace.
		getter, err := r.buildRESTClientGetter(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.GetStorageNamespace()))
		g.Expect(err).ToNot(HaveOccurred())

		store := helmstorage.Init(cfg.Driver)
		g.Expect(store.Create(rls)).To(Succeed())

		// Reconcile the Helm release.
		_, err = r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).ToNot(HaveOccurred())

		// Assert that the Helm release has been adopted.
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			release.ObservedToSnapshot(release.ObserveRelease(rls)),
		}))
		g.Expect(obj.Status.StorageNamespace).To(Equal(ns.Name))
		g.Expect(obj.Status.LastAttemptedConfigDigest).ToNot(BeEmpty())
		g.Expect(obj.Status.LastReleaseRevision).To(Equal(0))
	})

	t.Run("uninstalls HelmRelease if target has changed", func(t *testing.T) {
		g := NewWithT(t)

		chartMock := testutil.BuildChart()
		chartArtifact, err := testutil.SaveChartAsArtifact(chartMock, digest.SHA256, testServer.URL(), testServer.Root())
		g.Expect(err).ToNot(HaveOccurred())

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           chartArtifact,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				StorageNamespace: "other",
			},
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					{
						Name:      "mock",
						Namespace: "mock",
					},
				},
				HelmChart:        "mock/chart",
				StorageNamespace: "mock",
			},
		}

		c := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithStatusSubresource(&v2.HelmRelease{}).
			WithObjects(chart, obj).
			Build()

		r := &HelmReleaseReconciler{
			Client:           c,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, c), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeTrue())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallSucceededReason, "Release mock/mock.v0 was not found, assuming it is uninstalled"),
			*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason, "Release mock/mock.v0 was not found, assuming it is uninstalled"),
		}))

		// Verify history and storage namespace are cleared.
		g.Expect(obj.Status.History).To(BeNil())
		g.Expect(obj.Status.StorageNamespace).To(BeEmpty())
	})

	t.Run("resets failure counts on configuration change", func(t *testing.T) {
		g := NewWithT(t)

		chartMock := testutil.BuildChart()
		chartArtifact, err := testutil.SaveChartAsArtifact(chartMock, digest.SHA256, testServer.URL(), testServer.Root())
		g.Expect(err).ToNot(HaveOccurred())

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           chartArtifact,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "release",
				Namespace:  "mock",
				Generation: 2,
			},
			Spec: v2.HelmReleaseSpec{
				// Trigger a failure by setting an invalid storage namespace,
				// preventing the release from actually being installed.
				// This allows us to just test the failure count reset, without
				// having to facilitate a full install.
				StorageNamespace: "not-exist",
			},
			Status: v2.HelmReleaseStatus{
				HelmChart:       "mock/chart",
				InstallFailures: 2,
				UpgradeFailures: 3,
				Failures:        5,
				// Trigger actual failure reset due to change in spec.
				LastAttemptedGeneration: 1,
			},
		}

		c := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithStatusSubresource(&v2.HelmRelease{}).
			WithObjects(chart, obj).
			Build()

		r := &HelmReleaseReconciler{
			Client:           c,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		_, err = r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, c), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("namespaces \"not-exist\" not found"))

		// Verify failure counts are reset.
		g.Expect(obj.Status.InstallFailures).To(Equal(int64(0)))
		g.Expect(obj.Status.UpgradeFailures).To(Equal(int64(0)))
		g.Expect(obj.Status.Failures).To(Equal(int64(1)))
	})

	t.Run("sets last attempted values", func(t *testing.T) {
		g := NewWithT(t)

		chartMock := testutil.BuildChart()
		chartArtifact, err := testutil.SaveChartAsArtifact(chartMock, digest.SHA256, testServer.URL(), testServer.Root())
		g.Expect(err).ToNot(HaveOccurred())

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           chartArtifact,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "release",
				Namespace:  "mock",
				Generation: 2,
			},
			Spec: v2.HelmReleaseSpec{
				// Trigger a failure by setting an invalid storage namespace,
				// preventing the release from actually being installed.
				// This allows us to just test the values being set, without
				// having to facilitate a full install.
				StorageNamespace: "not-exist",
				Values: &apiextensionsv1.JSON{
					Raw: []byte(`{"foo":"bar"}`),
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart:          "mock/chart",
				ObservedGeneration: 2,
				// Confirm deprecated value is cleared.
				LastAttemptedValuesChecksum: "b5cbcf5c23cfd945d2cdf0ffaab387a46f2d054f",
			},
		}

		c := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithStatusSubresource(&v2.HelmRelease{}).
			WithObjects(chart, obj).
			Build()

		r := &HelmReleaseReconciler{
			Client:           c,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		_, err = r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, c), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("namespaces \"not-exist\" not found"))

		// Verify attempted values are set.
		g.Expect(obj.Status.LastAttemptedGeneration).To(Equal(obj.Generation))
		g.Expect(obj.Status.LastAttemptedRevision).To(Equal(chartMock.Metadata.Version))
		g.Expect(obj.Status.LastAttemptedConfigDigest).To(Equal("sha256:1dabc4e3cbbd6a0818bd460f3a6c9855bfe95d506c74726bc0f2edb0aecb1f4e"))
		g.Expect(obj.Status.LastAttemptedValuesChecksum).To(BeEmpty())
	})

	t.Run("error recovery updates ready condition", func(t *testing.T) {
		g := NewWithT(t)

		// Create a test namespace for storing the Helm release mock.
		ns, err := testEnv.CreateNamespace(context.TODO(), "error-recovery")
		g.Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), ns)
		})

		// Create HelmChart mock.
		chartMock := testutil.BuildChart()
		chartArtifact, err := testutil.SaveChartAsArtifact(chartMock, digest.SHA256, testServer.URL(), testServer.Root())
		g.Expect(err).ToNot(HaveOccurred())

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "error-recovery",
				Namespace:  ns.Name,
				Generation: 1,
			},
			Spec: sourcev1b2.HelmChartSpec{
				Chart:   "testdata/test-helmrepo",
				Version: "0.1.0",
				SourceRef: sourcev1b2.LocalHelmChartSourceReference{
					Kind: sourcev1b2.HelmRepositoryKind,
					Name: "error-recovery",
				},
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           chartArtifact,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		// Create a test Helm release storage mock.
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      "error-recovery",
			Namespace: ns.Name,
			Version:   1,
			Chart:     chartMock,
			Status:    helmrelease.StatusDeployed,
		}, testutil.ReleaseWithConfig(nil))

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "error-recovery",
				Namespace: ns.Name,
			},
			Spec: v2.HelmReleaseSpec{
				StorageNamespace: ns.Name,
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: chart.Namespace + "/" + chart.Name,
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(rls)),
				},
			},
		}

		c := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithStatusSubresource(&v2.HelmRelease{}).
			WithObjects(chart, obj).
			Build()

		r := &HelmReleaseReconciler{
			Client:           c,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
			FieldManager:     "test",
		}

		// Store the Helm release mock in the test namespace.
		getter, err := r.buildRESTClientGetter(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.GetStorageNamespace()))
		g.Expect(err).ToNot(HaveOccurred())

		store := helmstorage.Init(cfg.Driver)
		g.Expect(store.Create(rls)).To(Succeed())

		sp := patch.NewSerialPatcher(obj, r.Client)

		// List of failure reasons to test.
		prereqFailures := []string{
			v2.DependencyNotReadyReason,
			aclv1.AccessDeniedReason,
			v2.ArtifactFailedReason,
			"SourceNotReady",
			"ValuesError",
			"RESTClientError",
			"FactoryError",
		}

		// Update ready condition for each failure, reconcile and check if the
		// stale failure condition gets updated.
		for _, failReason := range prereqFailures {
			conditions.MarkFalse(obj, meta.ReadyCondition, failReason, "foo")
			err := sp.Patch(context.TODO(), obj,
				patch.WithOwnedConditions{Conditions: intreconcile.OwnedConditions},
				patch.WithFieldOwner(r.FieldManager),
			)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = r.reconcileRelease(context.TODO(), sp, obj)
			g.Expect(err).ToNot(HaveOccurred())

			ready := conditions.Get(obj, meta.ReadyCondition)
			g.Expect(ready.Status).To(Equal(metav1.ConditionUnknown))
			g.Expect(ready.Reason).To(Equal(meta.ProgressingReason))
		}
	})
}

func TestHelmReleaseReconciler_reconcileReleaseFromOCIRepositorySource(t *testing.T) {

	t.Run("handles chartRef and Chart definition failure", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
				Chart: v2.HelmChartTemplate{
					Spec: v2.HelmChartTemplateSpec{
						Chart: "mychart",
						SourceRef: v2.CrossNamespaceObjectReference{
							Name: "something",
						},
					},
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(obj).
				Build(),
		}

		res, err := r.Reconcile(context.TODO(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			},
		})

		// only chartRef or Chart must be set
		g.Expect(errors.Is(err, reconcile.TerminalError(fmt.Errorf("invalid Chart reference")))).To(BeTrue())
		g.Expect(res.IsZero()).To(BeTrue())
	})
	t.Run("handles ChartRef get failure", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(obj).
				Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		_, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, "Fulfilling prerequisites"),
			*conditions.FalseCondition(meta.ReadyCondition, v2.ArtifactFailedReason, "could not get Source object"),
		}))
	})

	t.Run("handles ACL error for ChartRef", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock-other",
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(obj).
				Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, reconcile.TerminalError(nil))).To(BeTrue())
		g.Expect(res.IsZero()).To(BeTrue())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.StalledCondition, acl.AccessDeniedReason, "cross-namespace references are not allowed"),
			*conditions.FalseCondition(meta.ReadyCondition, acl.AccessDeniedReason, "cross-namespace references are not allowed"),
		}))
	})

	t.Run("waits for ChartRef to have an Artifact", func(t *testing.T) {
		g := NewWithT(t)

		ocirepo := &sourcev1b2.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ocirepo",
				Namespace:  "mock",
				Generation: 2,
			},
			Status: sourcev1b2.OCIRepositoryStatus{
				ObservedGeneration: 2,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(ocirepo, obj).
				Build(),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForChart))
		g.Expect(res.RequeueAfter).To(Equal(obj.Spec.Interval.Duration))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, "SourceNotReady", "OCIRepository 'mock/ocirepo' is not ready"),
		}))
	})

	t.Run("reports values composition failure", func(t *testing.T) {
		g := NewWithT(t)

		ocirepo := &sourcev1b2.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ocirepo",
				Namespace:  "mock",
				Generation: 2,
			},
			Spec: sourcev1b2.OCIRepositorySpec{
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
			Status: sourcev1b2.OCIRepositoryStatus{
				ObservedGeneration: 2,
				Artifact:           &sourcev1.Artifact{},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
				ValuesFrom: []v2.ValuesReference{
					{
						Kind: "Secret",
						Name: "missing",
					},
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(ocirepo, obj).
				Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		_, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, "Fulfilling prerequisites"),
			*conditions.FalseCondition(meta.ReadyCondition, "ValuesError", "could not resolve Secret chart values reference 'mock/missing' with key 'values.yaml'"),
		}))
	})

	t.Run("reports Helm chart load failure", func(t *testing.T) {
		g := NewWithT(t)

		ocirepo := &sourcev1b2.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ocirepo",
				Namespace:  "mock",
				Generation: 2,
			},
			Spec: sourcev1b2.OCIRepositorySpec{
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
			Status: sourcev1b2.OCIRepositoryStatus{
				ObservedGeneration: 2,
				Artifact: &sourcev1.Artifact{
					URL: testServer.URL() + "/does-not-exist",
				},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(ocirepo, obj).
				Build(),
			requeueDependency: 10 * time.Second,
			EventRecorder:     record.NewFakeRecorder(32),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForDependency))
		g.Expect(res.RequeueAfter).To(Equal(r.requeueDependency))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, v2.ArtifactFailedReason, "Source not ready"),
		}))
	})
	t.Run("report helmChart load failure when switching from existing HelmChat to chartRef", func(t *testing.T) {
		g := NewWithT(t)

		chart := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "chart",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: sourcev1b2.HelmChartStatus{
				ObservedGeneration: 1,
				Artifact:           &sourcev1.Artifact{},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		ocirepo := &sourcev1b2.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ocirepo",
				Namespace:  "mock",
				Generation: 2,
			},
			Spec: sourcev1b2.OCIRepositorySpec{
				Interval: metav1.Duration{Duration: 1 * time.Second},
			},
			Status: sourcev1b2.OCIRepositoryStatus{
				ObservedGeneration: 2,
				Artifact: &sourcev1.Artifact{
					URL: testServer.URL() + "/does-not-exist",
				},
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
			},
			Status: v2.HelmReleaseStatus{
				HelmChart: "mock/chart",
			},
		}

		r := &HelmReleaseReconciler{
			Client: fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithStatusSubresource(&v2.HelmRelease{}).
				WithObjects(chart, ocirepo, obj).
				Build(),
			requeueDependency: 10 * time.Second,
			EventRecorder:     record.NewFakeRecorder(32),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, r.Client), obj)
		g.Expect(err).To(Equal(errWaitForDependency))
		g.Expect(res.RequeueAfter).To(Equal(r.requeueDependency))

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, v2.ArtifactFailedReason, "Source not ready"),
		}))
	})

	t.Run("handles reconciliation of new artifact", func(t *testing.T) {
		g := NewWithT(t)

		chartMock := testutil.BuildChart()
		chartArtifact, err := testutil.SaveChartAsArtifact(chartMock, digest.SHA256, testServer.URL(), testServer.Root())
		g.Expect(err).ToNot(HaveOccurred())

		ociRepo := &sourcev1b2.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ocirepo",
				Namespace:  "mock",
				Generation: 1,
			},
			Status: sourcev1b2.OCIRepositoryStatus{
				ObservedGeneration: 1,
				Artifact:           chartArtifact,
				Conditions: []metav1.Condition{
					{
						Type:   meta.ReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "release",
				Namespace: "mock",
			},
			Spec: v2.HelmReleaseSpec{
				ChartRef: &v2.CrossNamespaceSourceReference{
					Kind:      "OCIRepository",
					Name:      "ocirepo",
					Namespace: "mock",
				},
				StorageNamespace: "other",
			},
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					{
						Name:      "mock",
						Namespace: "mock",
					},
				},
				StorageNamespace: "mock",
			},
		}

		c := fake.NewClientBuilder().
			WithScheme(NewTestScheme()).
			WithStatusSubresource(&v2.HelmRelease{}).
			WithObjects(ociRepo, obj).
			Build()

		r := &HelmReleaseReconciler{
			Client:           c,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		res, err := r.reconcileRelease(context.TODO(), patch.NewSerialPatcher(obj, c), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeTrue())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(meta.ReconcilingCondition, meta.ProgressingReason, ""),
			*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallSucceededReason, "Release mock/mock.v0 was not found, assuming it is uninstalled"),
			*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason, "Release mock/mock.v0 was not found, assuming it is uninstalled"),
		}))

		// Verify history and storage namespace are cleared.
		g.Expect(obj.Status.History).To(BeNil())
		g.Expect(obj.Status.StorageNamespace).To(BeEmpty())
	})
}

func TestHelmReleaseReconciler_reconcileDelete(t *testing.T) {
	t.Run("uninstalls Helm release and removes chart", func(t *testing.T) {
		g := NewWithT(t)

		// Create a test namespace for storing the Helm release mock.
		ns, err := testEnv.CreateNamespace(context.TODO(), "reconcile-delete")
		g.Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), ns)
		})

		// Create HelmChart mock.
		hc := &sourcev1b2.HelmChart{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "reconcile-delete",
				Namespace: ns.Name,
			},
			Spec: sourcev1b2.HelmChartSpec{
				Chart:   "testdata/test-helmrepo",
				Version: "0.1.0",
				SourceRef: sourcev1b2.LocalHelmChartSourceReference{
					Kind: sourcev1b2.HelmRepositoryKind,
					Name: "reconcile-delete",
				},
			},
		}
		g.Expect(testEnv.Create(context.TODO(), hc)).To(Succeed())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), hc)
		})

		// Create a test Helm release storage mock.
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      "reconcile-delete",
			Namespace: ns.Name,
			Version:   1,
			Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
			Status:    helmrelease.StatusDeployed,
		})

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         ns.Name,
				Finalizers:        []string{v2.HelmReleaseFinalizer},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: ns.Name,
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(rls)),
				},
				HelmChart: hc.Namespace + "/" + hc.Name,
			},
		}

		r := &HelmReleaseReconciler{
			Client:           testEnv.Client,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		// Store the Helm release mock in the test namespace.
		getter, err := r.buildRESTClientGetter(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.Status.StorageNamespace))
		g.Expect(err).ToNot(HaveOccurred())

		store := helmstorage.Init(cfg.Driver)
		g.Expect(store.Create(rls)).To(Succeed())

		// Reconcile the actual deletion of the Helm release.
		res, err := r.reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		// Verify Helm release has been uninstalled.
		_, err = store.History(rls.Name)
		g.Expect(err).To(MatchError(helmdriver.ErrReleaseNotFound))

		// Verify Helm chart has been removed.
		g.Eventually(func(g Gomega) {
			err = testEnv.Get(context.TODO(), client.ObjectKey{
				Namespace: hc.Namespace,
				Name:      hc.Name,
			}, &sourcev1b2.HelmChart{})
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).Should(Succeed())
	})

	t.Run("removes finalizer for suspended resource with DeletionTimestamp", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers:        []string{v2.HelmReleaseFinalizer, "other-finalizer"},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
		}

		res, err := (&HelmReleaseReconciler{}).reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		g.Expect(obj.GetFinalizers()).To(ConsistOf("other-finalizer"))
	})

	t.Run("does not remove finalizer when DeletionTimestamp is not set", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: []string{v2.HelmReleaseFinalizer},
			},
			Spec: v2.HelmReleaseSpec{
				Suspend: true,
			},
		}

		res, err := (&HelmReleaseReconciler{}).reconcileDelete(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeTrue())

		g.Expect(obj.GetFinalizers()).To(ConsistOf(v2.HelmReleaseFinalizer))
	})
}

func TestHelmReleaseReconciler_reconcileReleaseDeletion(t *testing.T) {
	t.Run("uninstalls Helm release", func(t *testing.T) {
		g := NewWithT(t)

		// Create a test namespace for storing the Helm release mock.
		ns, err := testEnv.CreateNamespace(context.TODO(), "reconcile-release-deletion")
		g.Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), ns)
		})

		// Create a test Helm release storage mock.
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      "reconcile-delete",
			Namespace: ns.Name,
			Version:   1,
			Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
			Status:    helmrelease.StatusDeployed,
		})

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         ns.Name,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: ns.Name,
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(rls)),
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client:           testEnv.Client,
			GetClusterConfig: GetTestClusterConfig,
			EventRecorder:    record.NewFakeRecorder(32),
		}

		// Store the Helm release mock in the test namespace.
		getter, err := r.buildRESTClientGetter(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.Status.StorageNamespace))
		g.Expect(err).ToNot(HaveOccurred())

		store := helmstorage.Init(cfg.Driver)
		g.Expect(store.Create(rls)).To(Succeed())

		// Reconcile the actual deletion of the Helm release.
		err = r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify status of Helm release has been updated.
		g.Expect(obj.Status.StorageNamespace).To(BeEmpty())
		g.Expect(obj.Status.History).To(BeNil())

		// Verify Helm release has been uninstalled.
		_, err = store.History(rls.Name)
		g.Expect(err).To(MatchError(helmdriver.ErrReleaseNotFound))
	})

	t.Run("skip uninstalling Helm release when KubeConfig Secret is missing", func(t *testing.T) {
		g := NewWithT(t)

		// Create a test namespace for storing the Helm release mock.
		ns, err := testEnv.CreateNamespace(context.TODO(), "reconcile-release-deletion")
		g.Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), ns)
		})

		// Create a test Helm release storage mock.
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      "reconcile-delete",
			Namespace: ns.Name,
			Version:   1,
			Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
			Status:    helmrelease.StatusDeployed,
		})

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         ns.Name,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: ns.Name,
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(rls)),
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client:           testEnv.Client,
			GetClusterConfig: GetTestClusterConfig,
		}

		// Store the Helm release mock in the test namespace.
		getter, err := r.buildRESTClientGetter(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.Status.StorageNamespace))
		g.Expect(err).ToNot(HaveOccurred())

		store := helmstorage.Init(cfg.Driver)
		g.Expect(store.Create(rls)).To(Succeed())

		// Reconcile the actual deletion of the Helm release.
		obj.Spec.KubeConfig = &meta.KubeConfigReference{
			SecretRef: meta.SecretKeyReference{
				Name: "missing-secret",
			},
		}
		err = r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify status of Helm release has not been updated.
		g.Expect(obj.Status.StorageNamespace).ToNot(BeEmpty())
		g.Expect(obj.Status.History.Latest()).ToNot(BeNil())

		// Verify Helm release has not been uninstalled.
		_, err = store.History(rls.Name)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("error when REST client getter construction fails", func(t *testing.T) {
		g := NewWithT(t)

		mockErr := errors.New("mock error")
		r := &HelmReleaseReconciler{
			Client: testEnv.Client,
			GetClusterConfig: func() (*rest.Config, error) {
				return nil, mockErr
			},
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         "mock",
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "mock",
			},
		}

		// Reconcile the actual deletion of the Helm release.
		err := r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(errors.Is(err, mockErr)).To(BeTrue())
		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason,
				"failed to build REST client getter to uninstall release"),
		}))

		// Verify status of Helm release has not been updated.
		g.Expect(obj.Status.StorageNamespace).ToNot(BeEmpty())
	})

	t.Run("skip uninstalling Helm release when ServiceAccount is missing", func(t *testing.T) {
		g := NewWithT(t)

		// Create a test namespace for storing the Helm release mock.
		ns, err := testEnv.CreateNamespace(context.TODO(), "reconcile-release-deletion")
		g.Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), ns)
		})

		// Create a test Helm release storage mock.
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      "reconcile-delete",
			Namespace: ns.Name,
			Version:   1,
			Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
			Status:    helmrelease.StatusDeployed,
		})

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         ns.Name,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: ns.Name,
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(rls)),
				},
			},
		}

		r := &HelmReleaseReconciler{
			Client:           testEnv.Client,
			GetClusterConfig: GetTestClusterConfig,
		}

		// Store the Helm release mock in the test namespace.
		getter, err := r.buildRESTClientGetter(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.Status.StorageNamespace))
		g.Expect(err).ToNot(HaveOccurred())

		store := helmstorage.Init(cfg.Driver)
		g.Expect(store.Create(rls)).To(Succeed())

		// Reconcile the actual deletion of the Helm release.
		obj.Spec.ServiceAccountName = "missing-sa"
		err = r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify status of Helm release has not been updated.
		g.Expect(obj.Status.StorageNamespace).ToNot(BeEmpty())
		g.Expect(obj.Status.History.Latest()).ToNot(BeNil())

		// Verify Helm release has not been uninstalled.
		_, err = store.History(rls.Name)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("error when ServiceAccount existence check fails", func(t *testing.T) {
		g := NewWithT(t)

		var (
			serviceAccount = "missing-sa"
			namespace      = "mock"
			mockErr        = errors.New("mock error")
		)

		c := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == serviceAccount && key.Namespace == namespace {
					return mockErr
				}
				return client.Get(ctx, key, obj, opts...)
			},
		})

		r := &HelmReleaseReconciler{
			Client:           c.Build(),
			GetClusterConfig: GetTestClusterConfig,
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         namespace,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Spec: v2.HelmReleaseSpec{
				ServiceAccountName: serviceAccount,
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: namespace,
			},
		}

		// Reconcile the actual deletion of the Helm release.
		err := r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(errors.Is(err, mockErr)).To(BeTrue())
		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason,
				"failed to confirm ServiceAccount '%s' can be used to uninstall release", serviceAccount),
		}))

		// Verify status of Helm release has not been updated.
		g.Expect(obj.Status.StorageNamespace).ToNot(BeEmpty())
	})

	t.Run("error when Helm release uninstallation fails", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseReconciler{
			Client: testEnv.Client,
			GetClusterConfig: func() (*rest.Config, error) {
				return &rest.Config{
					Host: "https://failing-mock.local",
				}, nil
			},
			EventRecorder: record.NewFakeRecorder(32),
		}

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         "mock",
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "mock",
				History: v2.Snapshots{
					{},
				},
			},
		}

		err := r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason, "Kubernetes cluster unreachable"),
			*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallFailedReason, "Kubernetes cluster unreachable"),
		}))
	})

	t.Run("ignores ErrNoLatest when uninstalling Helm release", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         "mock",
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "mock",
			},
		}

		r := &HelmReleaseReconciler{
			Client:           testEnv.Client,
			GetClusterConfig: GetTestClusterConfig,
		}

		// Reconcile the actual deletion of the Helm release.
		err := r.reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify status of Helm release been updated.
		g.Expect(obj.Status.StorageNamespace).To(BeEmpty())
	})

	t.Run("error when DeletionTimestamp is not set", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "reconcile-delete",
				Namespace: "mock",
			},
		}

		err := (&HelmReleaseReconciler{}).reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("deletion timestamp is not set"))
	})

	t.Run("skip uninstalling Helm release when StorageNamespace is missing", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "reconcile-delete",
				Namespace:         "mock",
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		}

		err := (&HelmReleaseReconciler{}).reconcileReleaseDeletion(context.TODO(), obj)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestHelmReleaseReconciler_reconcileChartTemplate(t *testing.T) {
	t.Run("attempts to reconcile chart template", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseReconciler{
			Client:        fake.NewClientBuilder().WithScheme(NewTestScheme()).Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "default",
			},
		}

		// We do not care about the result of the reconcile, only that it was attempted.
		err := r.reconcileChartTemplate(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to run server-side apply"))
	})
}

func TestHelmReleaseReconciler_reconcileUninstall(t *testing.T) {
	t.Run("attempts to uninstall release", func(t *testing.T) {
		g := NewWithT(t)

		getter := kube.NewMemoryRESTClientGetter(testEnv.GetConfig())

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "default",
			},
		}

		// We do not care about the result of the uninstall, only that it was attempted.
		err := (&HelmReleaseReconciler{}).reconcileUninstall(context.TODO(), getter, obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, intreconcile.ErrNoLatest)).To(BeTrue())
	})

	t.Run("error on empty storage namespace", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "",
			},
		}

		err := (&HelmReleaseReconciler{}).reconcileUninstall(context.TODO(), nil, obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(conditions.IsFalse(obj, meta.ReadyCondition)).To(BeTrue())
		g.Expect(conditions.GetReason(obj, meta.ReadyCondition)).To(Equal("ConfigFactoryErr"))
		g.Expect(conditions.GetMessage(obj, meta.ReadyCondition)).To(ContainSubstring("no namespace provided"))
		g.Expect(obj.GetConditions()).To(HaveLen(1))
	})
}

func TestHelmReleaseReconciler_checkDependencies(t *testing.T) {
	tests := []struct {
		name    string
		obj     *v2.HelmRelease
		objects []client.Object
		expect  func(g *WithT, err error)
	}{
		{
			name: "all dependencies ready",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
						{
							Name:      "dependency-2",
							Namespace: "some-other-namespace",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
						Name:       "dependency-2",
						Namespace:  "some-other-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 2,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).ToNot(HaveOccurred())
			},
		},
		{
			name: "error on dependency with ObservedGeneration < Generation",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is not ready"))
			},
		},
		{
			name: "error on dependency with ObservedGeneration = Generation and ReadyCondition = False",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionFalse},
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is not ready"))
			},
		},
		{
			name: "error on dependency without conditions",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is not ready"))
			},
		},
		{
			name: "error on missing dependency",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithScheme(NewTestScheme())
			if len(tt.objects) > 0 {
				c.WithObjects(tt.objects...)
			}

			r := &HelmReleaseReconciler{
				Client: c.Build(),
			}

			err := r.checkDependencies(context.TODO(), tt.obj)
			tt.expect(g, err)
		})
	}
}

func TestHelmReleaseReconciler_adoptLegacyRelease(t *testing.T) {
	tests := []struct {
		name                      string
		releases                  func(namespace string) []*helmrelease.Release
		spec                      func(spec *v2.HelmReleaseSpec)
		status                    v2.HelmReleaseStatus
		expectHistory             func(releases []*helmrelease.Release) v2.Snapshots
		expectLastReleaseRevision int
		wantErr                   bool
	}{
		{
			name: "adopts last release revision",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      "orphaned",
						Namespace: namespace,
						Version:   6,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithTestHook()),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.ReleaseName = "orphaned"
			},
			status: v2.HelmReleaseStatus{
				LastReleaseRevision: 6,
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectLastReleaseRevision: 0,
		},
		{
			name: "includes test hooks if enabled",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      "orphaned-with-hooks",
						Namespace: namespace,
						Version:   3,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithTestHook()),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.ReleaseName = "orphaned-with-hooks"
				spec.Test = &v2.Test{
					Enable: true,
				}
			},
			status: v2.HelmReleaseStatus{
				LastReleaseRevision: 3,
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				snap := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				snap.SetTestHooks(release.TestHooksFromRelease(releases[0]))

				return v2.Snapshots{
					snap,
				}
			},
			expectLastReleaseRevision: 0,
		},
		{
			name: "non-existing release",
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.ReleaseName = "non-existing"
			},
			status: v2.HelmReleaseStatus{
				LastReleaseRevision: 2,
			},
			expectLastReleaseRevision: 2,
			wantErr:                   true,
		},
		{
			name: "without last release revision",
			status: v2.HelmReleaseStatus{
				LastReleaseRevision: 0,
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return nil
			},
			expectLastReleaseRevision: 0,
		},
		{
			name: "with existing history",
			status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					{
						Name: "something",
					},
				},
				LastReleaseRevision: 5,
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					{
						Name: "something",
					},
				}
			},
			expectLastReleaseRevision: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create a test namespace for storing the Helm release mock.
			ns, err := testEnv.CreateNamespace(context.TODO(), "adopt-release")
			g.Expect(err).ToNot(HaveOccurred())
			t.Cleanup(func() {
				_ = testEnv.Delete(context.TODO(), ns)
			})

			// Mock a HelmRelease object.
			obj := &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					StorageNamespace: ns.Name,
				},
				Status: tt.status,
			}
			if tt.spec != nil {
				tt.spec(&obj.Spec)
			}

			r := &HelmReleaseReconciler{
				Client:           testEnv.Client,
				GetClusterConfig: GetTestClusterConfig,
			}

			// Store the Helm release mock in the test namespace.
			getter, err := r.buildRESTClientGetter(context.TODO(), obj)
			g.Expect(err).ToNot(HaveOccurred())

			cfg, err := action.NewConfigFactory(getter, action.WithStorage(helmdriver.SecretsDriverName, obj.GetStorageNamespace()))
			g.Expect(err).ToNot(HaveOccurred())

			var releases []*helmrelease.Release
			if tt.releases != nil {
				releases = tt.releases(ns.Name)
			}
			store := helmstorage.Init(cfg.Driver)
			for _, rls := range releases {
				g.Expect(store.Create(rls)).To(Succeed())
			}

			// Adopt the Helm release mock.
			err = r.adoptLegacyRelease(context.TODO(), getter, obj)
			g.Expect(err != nil).To(Equal(tt.wantErr), "unexpected error: %s", err)

			// Verify the Helm release mock has been adopted.
			var expectHistory v2.Snapshots
			if tt.expectHistory != nil {
				expectHistory = tt.expectHistory(releases)
			}
			g.Expect(obj.Status.History).To(Equal(expectHistory))
			g.Expect(obj.Status.LastReleaseRevision).To(Equal(tt.expectLastReleaseRevision))
		})
	}
}

func TestHelmReleaseReconciler_buildRESTClientGetter(t *testing.T) {
	const (
		namespace = "some-namespace"
		kubeCfg   = `apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: https://1.2.3.4
  name: development
contexts:
- context:
    cluster: development
    namespace: frontend
    user: developer
  name: dev-frontend
current-context: dev-frontend
preferences: {}
users:
- name: developer
  user:
    password: some-password
    username: exp`
	)

	tests := []struct {
		name      string
		env       map[string]string
		getConfig func() (*rest.Config, error)
		spec      v2.HelmReleaseSpec
		secret    *corev1.Secret
		want      genericclioptions.RESTClientGetter
		wantErr   string
	}{
		{
			name: "builds in-cluster RESTClientGetter for HelmRelease",
			getConfig: func() (*rest.Config, error) {
				return clientcmd.RESTConfigFromKubeConfig([]byte(kubeCfg))
			},
			spec: v2.HelmReleaseSpec{},
			want: &kube.MemoryRESTClientGetter{},
		},
		{
			name: "returns error when in-cluster GetClusterConfig fails",
			getConfig: func() (*rest.Config, error) {
				return nil, errors.New("some-error")
			},
			wantErr: "some-error",
		},
		{
			name: "builds RESTClientGetter from HelmRelease with KubeConfig",
			spec: v2.HelmReleaseSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: "kubeconfig",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeconfig",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					kube.DefaultKubeConfigSecretKey: []byte(kubeCfg),
				},
			},
			want: &kube.MemoryRESTClientGetter{},
		},
		{
			name: "error on missing KubeConfig secret",
			spec: v2.HelmReleaseSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: "kubeconfig",
					},
				},
			},
			wantErr: "could not get KubeConfig secret",
		},
		{
			name: "error on invalid KubeConfig secret",
			spec: v2.HelmReleaseSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: "kubeconfig",
						Key:  "invalid-key",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeconfig",
					Namespace: namespace,
				},
			},
			wantErr: "does not contain a 'invalid-key' key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			c := fake.NewClientBuilder()
			if tt.secret != nil {
				c.WithObjects(tt.secret)
			}

			r := &HelmReleaseReconciler{
				Client:           c.Build(),
				GetClusterConfig: tt.getConfig,
			}

			getter, err := r.buildRESTClientGetter(context.Background(), &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-name",
					Namespace: namespace,
				},
				Spec: tt.spec,
			})
			if len(tt.wantErr) > 0 {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(getter).To(BeAssignableToTypeOf(tt.want))
			}
		})
	}
}

func TestHelmReleaseReconciler_getHelmChart(t *testing.T) {
	g := NewWithT(t)

	chart := &sourcev1b2.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "some-namespace",
			Name:      "some-chart-name",
		},
	}

	tests := []struct {
		name            string
		rel             *v2.HelmRelease
		chart           *sourcev1b2.HelmChart
		expectChart     bool
		wantErr         bool
		disallowCrossNS bool
	}{
		{
			name: "retrieves HelmChart object from Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       chart,
			expectChart: true,
		},
		{
			name: "no HelmChart found",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       nil,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "no HelmChart in Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "",
				},
			},
			chart:       chart,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "ACL disallows cross namespace",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:           chart,
			expectChart:     false,
			wantErr:         true,
			disallowCrossNS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder()
			c.WithScheme(NewTestScheme())
			if tt.chart != nil {
				c.WithObjects(tt.chart)
			}

			r := &HelmReleaseReconciler{
				Client:        c.Build(),
				EventRecorder: record.NewFakeRecorder(32),
			}

			curAllow := intacl.AllowCrossNamespaceRef
			intacl.AllowCrossNamespaceRef = !tt.disallowCrossNS
			t.Cleanup(func() { intacl.AllowCrossNamespaceRef = !curAllow })

			got, err := r.getSource(context.TODO(), tt.rel)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			hc, ok := got.(*sourcev1b2.HelmChart)
			g.Expect(ok).To(BeTrue())
			expect := g.Expect(hc.ObjectMeta)
			if tt.expectChart {
				expect.To(BeEquivalentTo(tt.chart.ObjectMeta))
			} else {
				expect.To(BeNil())
			}
		})
	}
}

func Test_waitForHistoryCacheSync(t *testing.T) {
	tests := []struct {
		name     string
		rel      *v2.HelmRelease
		cacheRel *v2.HelmRelease
		want     bool
	}{
		{
			name: "different history",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-name",
				},
				Status: v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{
							Version: 2,
							Status:  "deployed",
						},
						{
							Version: 1,
							Status:  "failed",
						},
					},
				},
			},
			cacheRel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-name",
				},
				Status: v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{
							Version: 1,
							Status:  "deployed",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "same history",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-name",
				},
				Status: v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{
							Version: 2,
							Status:  "deployed",
						},
						{
							Version: 1,
							Status:  "failed",
						},
					},
				},
			},
			cacheRel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-name",
				},
				Status: v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{
							Version: 2,
							Status:  "deployed",
						},
						{
							Version: 1,
							Status:  "failed",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "does not exist",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-name",
				},
			},
			cacheRel: nil,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder()
			c.WithScheme(NewTestScheme())
			if tt.cacheRel != nil {
				c.WithObjects(tt.cacheRel)
			}
			r := &HelmReleaseReconciler{
				Client: c.Build(),
			}

			got, err := r.waitForHistoryCacheSync(tt.rel)(context.Background())
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got == tt.want).To(BeTrue())
		})
	}
}

func TestValuesReferenceValidation(t *testing.T) {
	tests := []struct {
		name       string
		references []v2.ValuesReference
		wantErr    bool
	}{
		{
			name: "valid ValuesKey",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "any-key_na.me",
				},
			},
			wantErr: false,
		},
		{
			name: "valid ValuesKey: empty",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "",
				},
			},
			wantErr: false,
		},
		{
			name: "valid ValuesKey: long",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: strings.Repeat("a", 253),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid ValuesKey",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "a($&^%b",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid ValuesKey: too long",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: strings.Repeat("a", 254),
				},
			},
			wantErr: true,
		},
		{
			name: "valid target path: empty",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: "",
				},
			},
			wantErr: false,
		},
		{
			name: "valid target path",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: "list_with.nested-values.and.index[0]",
				},
			},
			wantErr: false,
		},
		{
			name: "valid target path: long",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: strings.Repeat("a", 250),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid target path: too long",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: strings.Repeat("a", 251),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid target path: opened index",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "a[",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid target path: incorrect index syntax",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "a]0[",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var values *apiextensionsv1.JSON
			v, _ := yaml.YAMLToJSON([]byte("values"))
			values = &apiextensionsv1.JSON{Raw: v}

			hr := v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: 5 * time.Minute},
					Chart: v2.HelmChartTemplate{
						Spec: v2.HelmChartTemplateSpec{
							Chart: "mychart",
							SourceRef: v2.CrossNamespaceObjectReference{
								Name: "something",
							},
						},
					},
					ValuesFrom: tt.references,
					Values:     values,
				},
			}

			err := testEnv.Create(context.TODO(), &hr, client.DryRunAll)
			if (err != nil) != tt.wantErr {
				t.Errorf("composeValues() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_isHelmChartReady(t *testing.T) {
	mock := &sourcev1b2.HelmChart{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HelmChart",
			APIVersion: sourcev1b2.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "mock",
			Namespace:  "default",
			Generation: 2,
		},
		Status: sourcev1b2.HelmChartStatus{
			ObservedGeneration: 2,
			Conditions: []metav1.Condition{
				{
					Type:   meta.ReadyCondition,
					Status: metav1.ConditionTrue,
				},
			},
			Artifact: &sourcev1.Artifact{},
		},
	}

	tests := []struct {
		name       string
		obj        *sourcev1b2.HelmChart
		want       bool
		wantReason string
	}{
		{
			name: "chart is ready",
			obj:  mock.DeepCopy(),
			want: true,
		},
		{
			name: "chart generation differs from observed generation while Ready=True",
			obj: func() *sourcev1b2.HelmChart {
				m := mock.DeepCopy()
				m.Generation = 3
				return m
			}(),
			want:       false,
			wantReason: "HelmChart 'default/mock' is not ready: latest generation of object has not been reconciled",
		},
		{
			name: "chart generation differs from observed generation while Ready=False",
			obj: func() *sourcev1b2.HelmChart {
				m := mock.DeepCopy()
				m.Generation = 3
				conditions.MarkFalse(m, meta.ReadyCondition, "Reason", "some reason")
				return m
			}(),
			want:       false,
			wantReason: "HelmChart 'default/mock' is not ready: some reason",
		},
		{
			name: "chart has Stalled=True",
			obj: func() *sourcev1b2.HelmChart {
				m := mock.DeepCopy()
				conditions.MarkFalse(m, meta.ReadyCondition, "Reason", "some reason")
				conditions.MarkStalled(m, "Reason", "some stalled reason")
				return m
			}(),
			want:       false,
			wantReason: "HelmChart 'default/mock' is not ready: some stalled reason",
		},
		{
			name: "chart does not have an Artifact",
			obj: func() *sourcev1b2.HelmChart {
				m := mock.DeepCopy()
				m.Status.Artifact = nil
				return m
			}(),
			want:       false,
			wantReason: "HelmChart 'default/mock' is not ready: does not have an artifact",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotReason := isReady(tt.obj, tt.obj.GetArtifact())
			if got != tt.want {
				t.Errorf("isHelmChartReady() got = %v, want %v", got, tt.want)
			}
			if gotReason != tt.wantReason {
				t.Errorf("isHelmChartReady() reason = %v, want %v", gotReason, tt.wantReason)
			}
		})
	}
}

func Test_isOCIRepositoryReady(t *testing.T) {
	mock := &sourcev1b2.OCIRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCIRepository",
			APIVersion: sourcev1b2.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "mock",
			Namespace:  "default",
			Generation: 2,
		},
		Status: sourcev1b2.OCIRepositoryStatus{
			ObservedGeneration: 2,
			Conditions: []metav1.Condition{
				{
					Type:   meta.ReadyCondition,
					Status: metav1.ConditionTrue,
				},
			},
			Artifact: &sourcev1.Artifact{},
		},
	}

	tests := []struct {
		name       string
		obj        *sourcev1b2.OCIRepository
		want       bool
		wantReason string
	}{
		{
			name: "OCIRepository is ready",
			obj:  mock.DeepCopy(),
			want: true,
		},
		{
			name: "OCIRepository generation differs from observed generation while Ready=True",
			obj: func() *sourcev1b2.OCIRepository {
				m := mock.DeepCopy()
				m.Generation = 3
				return m
			}(),
			want:       false,
			wantReason: "OCIRepository 'default/mock' is not ready: latest generation of object has not been reconciled",
		},
		{
			name: "OCIRepository generation differs from observed generation while Ready=False",
			obj: func() *sourcev1b2.OCIRepository {
				m := mock.DeepCopy()
				m.Generation = 3
				conditions.MarkFalse(m, meta.ReadyCondition, "Reason", "some reason")
				return m
			}(),
			want:       false,
			wantReason: "OCIRepository 'default/mock' is not ready: some reason",
		},
		{
			name: "OCIRepository has Stalled=True",
			obj: func() *sourcev1b2.OCIRepository {
				m := mock.DeepCopy()
				conditions.MarkFalse(m, meta.ReadyCondition, "Reason", "some reason")
				conditions.MarkStalled(m, "Reason", "some stalled reason")
				return m
			}(),
			want:       false,
			wantReason: "OCIRepository 'default/mock' is not ready: some stalled reason",
		},
		{
			name: "OCIRepository does not have an Artifact",
			obj: func() *sourcev1b2.OCIRepository {
				m := mock.DeepCopy()
				m.Status.Artifact = nil
				return m
			}(),
			want:       false,
			wantReason: "OCIRepository 'default/mock' is not ready: does not have an artifact",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotReason := isReady(tt.obj, tt.obj.GetArtifact())
			if got != tt.want {
				t.Errorf("isOCIRepositoryReady() got = %v, want %v", got, tt.want)
			}
			if gotReason != tt.wantReason {
				t.Errorf("isOCIRepositoryReady() reason = %v, want %v", gotReason, tt.wantReason)
			}
		})
	}
}
