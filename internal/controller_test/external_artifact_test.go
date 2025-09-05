/*
Copyright 2025 The Flux authors

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
	"testing"
	"time"

	aclv1 "github.com/fluxcd/pkg/apis/acl"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	v1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestExternalArtifact_LifeCycle(t *testing.T) {
	g := NewWithT(t)
	reconciler.AllowExternalArtifact = true

	// Create a namespace for the test
	ns := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-artifact-test",
		},
	}
	err := k8sClient.Create(context.Background(), &ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Create an ExternalArtifact containing a Helm chart
	revision := "1.0.0"
	eaKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      "test-ea",
	}
	ea, err := applyExternalArtifact(eaKey, revision)
	g.Expect(err).ToNot(HaveOccurred(), "failed to create ExternalArtifact")

	// Create a HelmRelease that references the ExternalArtifact
	hr := &v2.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v2.GroupVersion.String(),
			Kind:       v2.HelmReleaseKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: ns.Name,
		},
		Spec: v2.HelmReleaseSpec{
			ChartRef: &v2.CrossNamespaceSourceReference{
				Kind: sourcev1.ExternalArtifactKind,
				Name: ea.Name,
			},
			Interval: metav1.Duration{Duration: time.Hour},
		},
	}

	err = k8sClient.Create(context.Background(), hr)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("installs from external artifact", func(t *testing.T) {
		g.Eventually(func() bool {
			err = testEnv.Get(context.Background(), client.ObjectKeyFromObject(hr), hr)
			if err != nil {
				return false
			}
			return apimeta.IsStatusConditionTrue(hr.Status.Conditions, meta.ReadyCondition)
		}, 5*time.Second, time.Second).Should(BeTrue(), "HelmRelease did not become ready")

		g.Expect(hr.Status.LastAttemptedRevision).To(Equal(revision))
		g.Expect(hr.Status.LastAttemptedReleaseAction).To(Equal(v2.ReleaseActionInstall))
	})

	t.Run("upgrades at external artifact revision change", func(t *testing.T) {
		newRevision := "2.0.0"
		ea, err = applyExternalArtifact(eaKey, newRevision)
		g.Expect(err).ToNot(HaveOccurred())

		g.Eventually(func() bool {
			err = testEnv.Get(context.Background(), client.ObjectKeyFromObject(hr), hr)
			if err != nil {
				return false
			}
			return apimeta.IsStatusConditionTrue(hr.Status.Conditions, meta.ReadyCondition) &&
				hr.Status.LastAttemptedRevision == newRevision
		}, 5*time.Second, time.Second).Should(BeTrue(), "HelmRelease did not upgrade")

		g.Expect(hr.Status.LastAttemptedReleaseAction).To(Equal(v2.ReleaseActionUpgrade))
	})

	t.Run("fails when external artifact feature gate is disable", func(t *testing.T) {
		newRevision := "3.0.0"
		reconciler.AllowExternalArtifact = false

		ea, err = applyExternalArtifact(eaKey, newRevision)
		g.Expect(err).ToNot(HaveOccurred())

		g.Eventually(func() bool {
			err = testEnv.Get(context.Background(), client.ObjectKeyFromObject(hr), hr)
			if err != nil {
				return false
			}
			return apimeta.IsStatusConditionFalse(hr.Status.Conditions, meta.ReadyCondition)
		}, 5*time.Second, time.Second).Should(BeTrue())

		g.Expect(apimeta.IsStatusConditionTrue(hr.Status.Conditions, meta.StalledCondition)).Should(BeTrue())
		readyCondition := apimeta.FindStatusCondition(hr.Status.Conditions, meta.ReadyCondition)
		g.Expect(readyCondition.Reason).To(Equal(aclv1.AccessDeniedReason))
	})

	t.Run("uninstalls successfully", func(t *testing.T) {
		err = k8sClient.Delete(context.Background(), hr)
		g.Expect(err).ToNot(HaveOccurred())

		g.Eventually(func() bool {
			err = testEnv.Get(context.Background(), client.ObjectKeyFromObject(hr), hr)
			return err != nil && client.IgnoreNotFound(err) == nil
		}, 5*time.Second, time.Second).Should(BeTrue(), "HelmRelease was not deleted")
	})
}

func applyExternalArtifact(objKey client.ObjectKey, revision string) (*sourcev1.ExternalArtifact, error) {
	chart := testutil.BuildChart(testutil.ChartWithVersion(revision))
	artifact, err := testutil.SaveChartAsArtifact(chart, digest.SHA256, testServer.URL(), testServer.Root())
	if err != nil {
		return nil, err
	}

	ea := &sourcev1.ExternalArtifact{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sourcev1.GroupVersion.String(),
			Kind:       sourcev1.ExternalArtifactKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      objKey.Name,
			Namespace: objKey.Namespace,
		},
	}

	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner("kustomize-controller"),
	}

	err = k8sClient.Patch(context.Background(), ea, client.Apply, patchOpts...)
	if err != nil {
		return nil, err
	}

	ea.ManagedFields = nil
	ea.Status = sourcev1.ExternalArtifactStatus{
		Artifact: artifact,
		Conditions: []metav1.Condition{
			{
				Type:               meta.ReadyCondition,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             meta.SucceededReason,
				ObservedGeneration: 1,
			},
		},
	}

	statusOpts := &client.SubResourcePatchOptions{
		PatchOptions: client.PatchOptions{
			FieldManager: "source-controller",
		},
	}

	err = k8sClient.Status().Patch(context.Background(), ea, client.Apply, statusOpts)
	if err != nil {
		return nil, err
	}
	return ea, nil
}
