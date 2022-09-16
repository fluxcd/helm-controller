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

package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestReleaseStrategy_CleanRelease_MustContinue(t *testing.T) {
	tests := []struct {
		name     string
		current  ReconcilerType
		previous ReconcilerTypeSet
		want     bool
	}{
		{
			name:    "continue if not in previous",
			current: ReconcilerTypeRemediate,
			previous: []ReconcilerType{
				ReconcilerTypeRelease,
			},
			want: true,
		},
		{
			name:    "do not continue if in previous",
			current: ReconcilerTypeRemediate,
			previous: []ReconcilerType{
				ReconcilerTypeRemediate,
			},
			want: false,
		},
		{
			name:     "do continue on nil",
			current:  ReconcilerTypeRemediate,
			previous: nil,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := &cleanReleaseStrategy{}
			if got := at.MustContinue(tt.current, tt.previous); got != tt.want {
				g := NewWithT(t)
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestReleaseStrategy_CleanRelease_MustStop(t *testing.T) {
	tests := []struct {
		name     string
		current  ReconcilerType
		previous ReconcilerTypeSet
		want     bool
	}{
		{
			name:    "stop if current is remediate",
			current: ReconcilerTypeRemediate,
			want:    true,
		},
		{
			name:    "do not stop if current is not remediate",
			current: ReconcilerTypeRelease,
			want:    false,
		},
		{
			name:    "do not stop if current is not remediate",
			current: ReconcilerTypeUnlock,
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := &cleanReleaseStrategy{}
			if got := at.MustStop(tt.current, tt.previous); got != tt.want {
				g := NewWithT(t)
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestAtomicRelease_Reconcile(t *testing.T) {
	t.Run("runs a series of actions", func(t *testing.T) {
		g := NewWithT(t)

		namedNS, err := testEnv.CreateNamespace(context.TODO(), mockReleaseNamespace)
		g.Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), namedNS)
		})
		releaseNamespace := namedNS.Name

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mockReleaseName,
				Namespace: releaseNamespace,
			},
			Spec: v2.HelmReleaseSpec{
				ReleaseName:     mockReleaseName,
				TargetNamespace: releaseNamespace,
				Test: &v2.Test{
					Enable: true,
				},
				StorageNamespace: releaseNamespace,
				Timeout:          &metav1.Duration{Duration: 100 * time.Millisecond},
			},
		}

		getter, err := RESTClientGetterFromManager(testEnv.Manager, obj.GetReleaseNamespace())
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter,
			action.WithStorage(action.DefaultStorageDriver, obj.GetStorageNamespace()),
			action.WithDebugLog(logr.Discard()),
		)
		g.Expect(err).ToNot(HaveOccurred())

		client := fake.NewClientBuilder().
			WithScheme(testEnv.Scheme()).
			WithObjects(obj).
			WithStatusSubresource(&v2.HelmRelease{}).
			Build()
		recorder := record.NewFakeRecorder(10)

		req := &Request{
			Object: obj,
			Chart:  testutil.BuildChart(testutil.ChartWithTestHook()),
			Values: nil,
		}
		g.Expect(NewAtomicRelease(client, cfg, recorder).Reconcile(context.TODO(), req)).ToNot(HaveOccurred())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			{
				Type:    meta.ReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v2.TestSucceededReason,
				Message: "test hook completed successfully",
			},
			{
				Type:    v2.ReleasedCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v2.InstallSucceededReason,
				Message: "Installed release",
			},
			{
				Type:    v2.TestSuccessCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v2.TestSucceededReason,
				Message: "test hook completed successfully",
			},
		}))
		g.Expect(obj.GetCurrent()).ToNot(BeNil(), "expected current to not be nil")
		g.Expect(obj.GetPrevious()).To(BeNil(), "expected previous to be nil")

		g.Expect(obj.Status.Failures).To(BeZero())
		g.Expect(obj.Status.InstallFailures).To(BeZero())
		g.Expect(obj.Status.UpgradeFailures).To(BeZero())

		g.Expect(NextAction(context.TODO(), cfg, recorder, req)).To(And(Succeed(), BeNil()))
	})
}
