/*
Copyright 2026 The Flux authors

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

package controller_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestHelmReleaseReconciler_WaitsForCustomHealthChecks(t *testing.T) {
	g := NewWithT(t)
	id := "cel-" + randStringRunes(5)
	timeout := 60 * time.Second

	err := createNamespace(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create test namespace")

	err = createKubeConfigSecret(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create kubeconfig secret")

	// Create a Helm chart that deploys a ConfigMap
	chart := testutil.BuildChart(
		testutil.ChartWithVersion("1.0.0"),
		testutil.ChartWithName("test-cel-chart"),
	)
	chartArtifact, err := testutil.SaveChartAsArtifact(chart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	chartKey := types.NamespacedName{
		Name:      fmt.Sprintf("cel-%s", randStringRunes(5)),
		Namespace: id,
	}

	err = applyHelmChart(chartKey, chartArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	hrKey := types.NamespacedName{
		Name:      fmt.Sprintf("cel-%s", randStringRunes(5)),
		Namespace: id,
	}

	hr := &v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hrKey.Name,
			Namespace: hrKey.Namespace,
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			ChartRef: &v2.CrossNamespaceSourceReference{
				Kind:      sourcev1.HelmChartKind,
				Name:      chartKey.Name,
				Namespace: chartKey.Namespace,
			},
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: &meta.SecretKeyReference{
					Name: "kubeconfig",
				},
			},
			TargetNamespace: id,
			Timeout:         &metav1.Duration{Duration: 30 * time.Second},
			// Use a CEL expression that references a non-existent field
			// This will fail because 'data.foo.bar' doesn't exist
			HealthCheckExprs: []kustomize.CustomHealthCheck{{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				HealthCheckExpressions: kustomize.HealthCheckExpressions{
					InProgress: "has(data.foo.bar)",
					Current:    "true",
				},
			}},
		},
	}

	err = k8sClient.Create(context.Background(), hr)
	g.Expect(err).NotTo(HaveOccurred())

	resultHR := &v2.HelmRelease{}
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return apimeta.IsStatusConditionFalse(resultHR.Status.Conditions, meta.ReadyCondition)
	}, timeout, time.Second).Should(BeTrue())

	readyCondition := apimeta.FindStatusCondition(resultHR.Status.Conditions, meta.ReadyCondition)
	g.Expect(readyCondition).NotTo(BeNil())
	// The health check should fail with the CEL expression returning Unknown status
	// because the expression tries to access 'data.foo.bar' which doesn't exist.
	g.Expect(readyCondition.Message).
		To(ContainSubstring("failed to evaluate the CEL expression"))
}

func TestHelmReleaseReconciler_CancelHealthCheckOnNewRevision(t *testing.T) {
	g := NewWithT(t)
	id := "cancel-" + randStringRunes(5)
	timeout := 120 * time.Second

	err := createNamespace(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create test namespace")

	err = createKubeConfigSecret(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create kubeconfig secret")

	// Create initial successful chart
	successChart := testutil.BuildChart(
		testutil.ChartWithVersion("1.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
	)
	successArtifact, err := testutil.SaveChartAsArtifact(successChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	chartKey := types.NamespacedName{
		Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
		Namespace: id,
	}

	err = applyHelmChart(chartKey, successArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	hrKey := types.NamespacedName{
		Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
		Namespace: id,
	}

	hr := &v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hrKey.Name,
			Namespace: hrKey.Namespace,
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			ChartRef: &v2.CrossNamespaceSourceReference{
				Kind:      sourcev1.HelmChartKind,
				Name:      chartKey.Name,
				Namespace: chartKey.Namespace,
			},
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: &meta.SecretKeyReference{
					Name: "kubeconfig",
				},
			},
			TargetNamespace: id,
			Timeout:         &metav1.Duration{Duration: 5 * time.Minute},
		},
	}

	err = k8sClient.Create(context.Background(), hr)
	g.Expect(err).NotTo(HaveOccurred())

	// Wait for initial reconciliation to succeed
	resultHR := &v2.HelmRelease{}
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready")

	// Create a failing chart (deployment with bad image that will timeout)
	failingChart := testutil.BuildChart(
		testutil.ChartWithVersion("2.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
		testutil.ChartWithFailingDeployment(),
	)
	failingArtifact, err := testutil.SaveChartAsArtifact(failingChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	// Apply failing revision
	err = applyHelmChart(chartKey, failingArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	// Wait for reconciliation to start on failing revision
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return resultHR.Status.LastAttemptedRevision == failingChart.Metadata.Version
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not start reconciling failing revision")

	// Now quickly apply a fixed revision while health check should be in progress
	fixedChart := testutil.BuildChart(
		testutil.ChartWithVersion("3.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
	)
	fixedArtifact, err := testutil.SaveChartAsArtifact(fixedChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	// Give some time for health check to start
	time.Sleep(2 * time.Second)

	// Apply the fixed revision
	err = applyHelmChart(chartKey, fixedArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	// The key test: verify that the fixed revision gets attempted
	// and that the health check cancellation worked
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return resultHR.Status.LastAttemptedRevision == fixedChart.Metadata.Version
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not attempt the fixed revision")

	// Verify the HealthCheckCanceled event was emitted.
	g.Eventually(func() bool {
		events := getEvents(resultHR.GetName(), nil)
		for _, event := range events {
			if event.Reason == meta.HealthCheckCanceledReason {
				t.Logf("Found HealthCheckCanceled event: %s", event.Message)
				return true
			}
		}
		return false
	}, timeout, time.Second).Should(BeTrue(), "HealthCheckCanceled event should be recorded")

	// Verify the event message indicates the trigger source.
	events := getEvents(resultHR.GetName(), nil)
	var cancelEvent *corev1.Event
	for i := range events {
		if events[i].Reason == meta.HealthCheckCanceledReason {
			cancelEvent = &events[i]
			break
		}
	}
	g.Expect(cancelEvent).ToNot(BeNil())
	g.Expect(cancelEvent.Message).To(ContainSubstring("Health checks canceled"))
	g.Expect(cancelEvent.Message).To(ContainSubstring("HelmChart"))

	// Verify the HelmRelease becomes Ready after the fixed revision is reconciled.
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready after fixed revision")
}

func TestHelmReleaseReconciler_CancelHealthCheckWithRemediation(t *testing.T) {
	g := NewWithT(t)
	id := "cancel-" + randStringRunes(5)
	timeout := 120 * time.Second

	err := createNamespace(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create test namespace")

	err = createKubeConfigSecret(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create kubeconfig secret")

	// Create initial successful chart
	successChart := testutil.BuildChart(
		testutil.ChartWithVersion("1.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
	)
	successArtifact, err := testutil.SaveChartAsArtifact(successChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	chartKey := types.NamespacedName{
		Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
		Namespace: id,
	}

	err = applyHelmChart(chartKey, successArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	hr := &v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
			Namespace: id,
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			ChartRef: &v2.CrossNamespaceSourceReference{
				Kind:      sourcev1.HelmChartKind,
				Name:      chartKey.Name,
				Namespace: chartKey.Namespace,
			},
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: &meta.SecretKeyReference{
					Name: "kubeconfig",
				},
			},
			TargetNamespace: id,
			Timeout:         &metav1.Duration{Duration: 5 * time.Minute},
			Upgrade: &v2.Upgrade{
				Remediation: &v2.UpgradeRemediation{
					Retries:              1,
					RemediateLastFailure: new(true),
				},
			},
		},
	}

	err = k8sClient.Create(context.Background(), hr)
	g.Expect(err).NotTo(HaveOccurred())

	// Wait for initial reconciliation to succeed
	resultHR := &v2.HelmRelease{}
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready")

	// Create a failing chart (deployment with bad image that will timeout)
	failingChart := testutil.BuildChart(
		testutil.ChartWithVersion("2.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
		testutil.ChartWithFailingDeployment(),
	)
	failingArtifact, err := testutil.SaveChartAsArtifact(failingChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	// Apply failing revision
	err = applyHelmChart(chartKey, failingArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	// releaseStatuses returns the status of each Helm release in storage
	releaseStatuses := func() map[string]string {
		secrets := &corev1.SecretList{}
		_ = k8sClient.List(context.Background(), secrets, client.InNamespace(id), client.MatchingLabels{"owner": "helm"})
		statuses := map[string]string{}
		for _, secret := range secrets.Items {
			statuses[secret.Labels["version"]] = secret.Labels["status"]
		}
		return statuses
	}

	// Wait for the failing revision to be pending while its health check runs
	g.Eventually(func() string {
		return releaseStatuses()["2"]
	}, timeout, time.Second).Should(Equal("pending-upgrade"), "failing revision did not start upgrading")

	// Apply a fixed revision while the upgrade is in progress
	fixedChart := testutil.BuildChart(
		testutil.ChartWithVersion("3.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
	)
	fixedArtifact, err := testutil.SaveChartAsArtifact(fixedChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	err = applyHelmChart(chartKey, fixedArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	// Wait for the fixed revision to be released
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return resultHR.Status.LastAttemptedRevision == fixedChart.Metadata.Version &&
			apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready after fixed revision")

	// The key test: no rollback release is stored for the canceled upgrade
	g.Expect(releaseStatuses()).To(Equal(map[string]string{
		"1": "superseded",
		"2": "failed",
		"3": "deployed",
	}), "unexpected releases in Helm storage")

	latest := resultHR.Status.History.Latest()
	g.Expect(latest).ToNot(BeNil())
	g.Expect(latest.Version).To(Equal(3))
	g.Expect(latest.Action).To(Equal(v2.ReleaseActionUpgrade))
	g.Expect(latest.ChartVersion).To(Equal(fixedChart.Metadata.Version))

	// Verify the HealthCheckCanceled event was emitted.
	g.Eventually(func() bool {
		for _, event := range getEvents(resultHR.GetName(), nil) {
			if event.Reason == meta.HealthCheckCanceledReason {
				return true
			}
		}
		return false
	}, timeout, time.Second).Should(BeTrue(), "HealthCheckCanceled event should be recorded")
}

func TestHelmReleaseReconciler_CancelHealthCheckWithRemediationOnReconcileRequest(t *testing.T) {
	g := NewWithT(t)
	id := "cancel-" + randStringRunes(5)
	timeout := 120 * time.Second

	err := createNamespace(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create test namespace")

	err = createKubeConfigSecret(id)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create kubeconfig secret")

	// Create initial successful chart
	successChart := testutil.BuildChart(
		testutil.ChartWithVersion("1.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
	)
	successArtifact, err := testutil.SaveChartAsArtifact(successChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	chartKey := types.NamespacedName{
		Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
		Namespace: id,
	}

	err = applyHelmChart(chartKey, successArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	hr := &v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
			Namespace: id,
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			ChartRef: &v2.CrossNamespaceSourceReference{
				Kind:      sourcev1.HelmChartKind,
				Name:      chartKey.Name,
				Namespace: chartKey.Namespace,
			},
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: &meta.SecretKeyReference{
					Name: "kubeconfig",
				},
			},
			TargetNamespace: id,
			Timeout:         &metav1.Duration{Duration: 5 * time.Minute},
			Upgrade: &v2.Upgrade{
				// Without retries the rollback is the last action, so no
				// further upgrade attempt changes the history afterwards.
				Remediation: &v2.UpgradeRemediation{
					Retries:              0,
					RemediateLastFailure: new(true),
				},
			},
		},
	}

	err = k8sClient.Create(context.Background(), hr)
	g.Expect(err).NotTo(HaveOccurred())

	// Wait for initial reconciliation to succeed
	resultHR := &v2.HelmRelease{}
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready")

	// Create a failing chart (deployment with bad image that will timeout)
	failingChart := testutil.BuildChart(
		testutil.ChartWithVersion("2.0.0"),
		testutil.ChartWithName("test-cancel-chart"),
		testutil.ChartWithFailingDeployment(),
	)
	failingArtifact, err := testutil.SaveChartAsArtifact(failingChart, digest.SHA256, testServer.URL(), testServer.Root())
	g.Expect(err).NotTo(HaveOccurred())

	// Apply failing revision
	err = applyHelmChart(chartKey, failingArtifact)
	g.Expect(err).NotTo(HaveOccurred())

	// releaseStatuses returns the status of each Helm release in storage
	releaseStatuses := func() map[string]string {
		secrets := &corev1.SecretList{}
		_ = k8sClient.List(context.Background(), secrets, client.InNamespace(id), client.MatchingLabels{"owner": "helm"})
		statuses := map[string]string{}
		for _, secret := range secrets.Items {
			statuses[secret.Labels["version"]] = secret.Labels["status"]
		}
		return statuses
	}

	// Wait for the failing revision to be pending while its health check runs
	g.Eventually(func() string {
		return releaseStatuses()["2"]
	}, timeout, time.Second).Should(Equal("pending-upgrade"), "failing revision did not start upgrading")

	// Request a reconciliation without changing the chart or values
	err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
	g.Expect(err).NotTo(HaveOccurred())
	patch := client.MergeFrom(resultHR.DeepCopy())
	resultHR.SetAnnotations(map[string]string{
		meta.ReconcileRequestAnnotation: time.Now().Format(time.RFC3339Nano),
	})
	err = k8sClient.Patch(context.Background(), resultHR, patch)
	g.Expect(err).NotTo(HaveOccurred())

	// Wait for the failing revision to be remediated
	g.Eventually(func() bool {
		_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
		return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, v2.RemediatedCondition)
	}, timeout, time.Second).Should(BeTrue(), "HelmRelease was not remediated")

	// The key test: the next reconcile rolls back on a fresh context
	g.Expect(releaseStatuses()).To(Equal(map[string]string{
		"1": "superseded",
		"2": "failed",
		"3": "deployed",
	}), "unexpected releases in Helm storage")

	latest := resultHR.Status.History.Latest()
	g.Expect(latest).ToNot(BeNil())
	g.Expect(latest.Version).To(Equal(3))
	g.Expect(latest.Action).To(Equal(v2.ReleaseActionRollback))
	g.Expect(latest.ChartVersion).To(Equal(successChart.Metadata.Version))

	// Verify the HealthCheckCanceled event was emitted.
	g.Eventually(func() bool {
		for _, event := range getEvents(resultHR.GetName(), nil) {
			if event.Reason == meta.HealthCheckCanceledReason {
				return true
			}
		}
		return false
	}, timeout, time.Second).Should(BeTrue(), "HealthCheckCanceled event should be recorded")
}

func TestHelmReleaseReconciler_CancelHealthCheckWithUninstallRemediation(t *testing.T) {
	tests := []struct {
		name string
		// failUpgrade fails an upgrade of a ready release, remediated with the
		// configured uninstall strategy, instead of the first install.
		failUpgrade bool
		// reconcileRequest requests a reconciliation without changing the
		// chart or values, instead of applying a fixed chart revision.
		reconcileRequest bool
		wantReleases     map[string]string
		wantVersion      int
		wantAction       v2.ReleaseAction
		wantChartVersion string
		wantUninstall    bool
		wantRemediated   bool
	}{
		{
			name:             "install on new revision",
			wantReleases:     map[string]string{"1": "superseded", "2": "deployed"},
			wantVersion:      2,
			wantAction:       v2.ReleaseActionUpgrade,
			wantChartVersion: "2.0.0",
		},
		{
			name:             "install on reconcile request",
			reconcileRequest: true,
			wantReleases:     map[string]string{},
			wantVersion:      1,
			wantAction:       v2.ReleaseActionUninstallRemediation,
			wantChartVersion: "1.0.0",
			wantUninstall:    true,
			wantRemediated:   true,
		},
		{
			name:             "upgrade on new revision",
			failUpgrade:      true,
			wantReleases:     map[string]string{"1": "superseded", "2": "failed", "3": "deployed"},
			wantVersion:      3,
			wantAction:       v2.ReleaseActionUpgrade,
			wantChartVersion: "3.0.0",
		},
		{
			// After the uninstall the controller installs the failing chart
			// again as a new release, which fails too, so the latest release
			// is that install.
			name:             "upgrade on reconcile request",
			failUpgrade:      true,
			reconcileRequest: true,
			wantReleases:     map[string]string{"1": "failed"},
			wantVersion:      1,
			wantAction:       v2.ReleaseActionInstall,
			wantChartVersion: "2.0.0",
			wantUninstall:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			id := "cancel-" + randStringRunes(5)
			timeout := 120 * time.Second

			err := createNamespace(id)
			g.Expect(err).NotTo(HaveOccurred(), "failed to create test namespace")

			err = createKubeConfigSecret(id)
			g.Expect(err).NotTo(HaveOccurred(), "failed to create kubeconfig secret")

			chartKey := types.NamespacedName{
				Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
				Namespace: id,
			}

			// applyChart applies a chart revision, with a deployment that
			// never becomes ready if failing is set.
			applyChart := func(version string, failing bool) {
				opts := []testutil.ChartOption{
					testutil.ChartWithVersion(version),
					testutil.ChartWithName("test-cancel-chart"),
				}
				if failing {
					opts = append(opts, testutil.ChartWithFailingDeployment())
				}
				artifact, err := testutil.SaveChartAsArtifact(testutil.BuildChart(opts...), digest.SHA256, testServer.URL(), testServer.Root())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(applyHelmChart(chartKey, artifact)).To(Succeed())
			}

			// releaseStatuses returns the status of each Helm release in storage
			releaseStatuses := func() map[string]string {
				secrets := &corev1.SecretList{}
				_ = k8sClient.List(context.Background(), secrets, client.InNamespace(id), client.MatchingLabels{"owner": "helm"})
				statuses := map[string]string{}
				for _, secret := range secrets.Items {
					statuses[secret.Labels["version"]] = secret.Labels["status"]
				}
				return statuses
			}

			hr := &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("cancel-%s", randStringRunes(5)),
					Namespace: id,
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: 10 * time.Minute},
					ChartRef: &v2.CrossNamespaceSourceReference{
						Kind:      sourcev1.HelmChartKind,
						Name:      chartKey.Name,
						Namespace: chartKey.Namespace,
					},
					KubeConfig: &meta.KubeConfigReference{
						SecretRef: &meta.SecretKeyReference{
							Name: "kubeconfig",
						},
					},
					TargetNamespace: id,
					Timeout:         &metav1.Duration{Duration: 5 * time.Minute},
				},
			}

			// Without retries, the uninstall only runs when the remediation
			// of the last failure is enabled.
			if tt.failUpgrade {
				hr.Spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Strategy:             new(v2.UninstallRemediationStrategy),
						RemediateLastFailure: new(true),
					},
				}
				hr.Spec.Install = &v2.Install{
					// The failing chart is installed again after the uninstall.
					// Let that install fail after 10s instead of 5m.
					Timeout: &metav1.Duration{Duration: 10 * time.Second},
				}
			} else {
				hr.Spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						RemediateLastFailure: new(true),
					},
				}
			}

			applyChart("1.0.0", !tt.failUpgrade)
			err = k8sClient.Create(context.Background(), hr)
			g.Expect(err).NotTo(HaveOccurred())

			resultHR := &v2.HelmRelease{}
			fixedVersion := "2.0.0"
			pendingReleases := map[string]string{"1": "pending-install"}
			if tt.failUpgrade {
				// Wait for initial reconciliation to succeed
				g.Eventually(func() bool {
					_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
					return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
				}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready")

				applyChart("2.0.0", true)
				fixedVersion = "3.0.0"
				pendingReleases = map[string]string{"1": "deployed", "2": "pending-upgrade"}
			}

			// Wait for the failing release to be pending while its health check runs
			g.Eventually(releaseStatuses, timeout, time.Second).Should(Equal(pendingReleases),
				"failing release did not start")

			if tt.reconcileRequest {
				// Request a reconciliation without changing the chart or values
				err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
				g.Expect(err).NotTo(HaveOccurred())
				patch := client.MergeFrom(resultHR.DeepCopy())
				resultHR.SetAnnotations(map[string]string{
					meta.ReconcileRequestAnnotation: time.Now().Format(time.RFC3339Nano),
				})
				err = k8sClient.Patch(context.Background(), resultHR, patch)
				g.Expect(err).NotTo(HaveOccurred())

				// Wait for the HelmRelease to stall with no retries left
				g.Eventually(func() bool {
					_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
					return apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.StalledCondition)
				}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not stall")
			} else {
				// Apply a fixed revision while the health check runs
				applyChart(fixedVersion, false)

				// Wait for the fixed revision to be released
				g.Eventually(func() bool {
					_ = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)
					return resultHR.Status.LastAttemptedRevision == fixedVersion &&
						apimeta.IsStatusConditionTrue(resultHR.Status.Conditions, meta.ReadyCondition)
				}, timeout, time.Second).Should(BeTrue(), "HelmRelease did not become ready after fixed revision")
			}

			// The key test: the releases in Helm storage show whether the
			// failing release was uninstalled
			g.Expect(releaseStatuses()).To(Equal(tt.wantReleases), "unexpected releases in Helm storage")

			// The history is patched after the conditions, so wait for it
			g.Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(hr), resultHR)).To(Succeed())
				latest := resultHR.Status.History.Latest()
				g.Expect(latest).ToNot(BeNil())
				g.Expect(latest.Version).To(Equal(tt.wantVersion))
				g.Expect(latest.Action).To(Equal(tt.wantAction))
				g.Expect(latest.ChartVersion).To(Equal(tt.wantChartVersion))
			}, timeout, time.Second).Should(Succeed())

			remediated := apimeta.FindStatusCondition(resultHR.Status.Conditions, v2.RemediatedCondition)
			if tt.wantRemediated {
				g.Expect(remediated).ToNot(BeNil())
				g.Expect(remediated.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(remediated.Reason).To(Equal(v2.UninstallSucceededReason))
			} else {
				g.Expect(remediated).To(BeNil())
			}

			// Verify the HealthCheckCanceled event was emitted, and whether
			// the failing release was uninstalled.
			eventReasons := func() []string {
				var reasons []string
				for _, event := range getEvents(hr.GetName(), nil) {
					reasons = append(reasons, event.Reason)
				}
				return reasons
			}
			g.Eventually(eventReasons, timeout, time.Second).Should(ContainElement(meta.HealthCheckCanceledReason),
				"HealthCheckCanceled event should be recorded")
			if tt.wantUninstall {
				g.Eventually(eventReasons, timeout, time.Second).Should(ContainElement(v2.UninstallSucceededReason),
					"UninstallSucceeded event should be recorded")
			} else {
				g.Expect(eventReasons()).ToNot(ContainElement(v2.UninstallSucceededReason))
			}
			g.Expect(eventReasons()).ToNot(ContainElement(v2.UninstallFailedReason))
		})
	}
}
