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
			Timeout:         &metav1.Duration{Duration: 5 * time.Second},
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
		To(ContainSubstring("not ready"))
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
}
