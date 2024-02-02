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
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/ssa"
	"github.com/fluxcd/pkg/ssa/jsondiff"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
)

// CorrectClusterDrift is a reconciler that attempts to correct the cluster state
// of a Helm release. It does so by applying the Helm release's desired state
// to the cluster based on a jsondiff.DiffSet.
//
// The reconciler will only attempt to correct the cluster state if the Helm
// release has drift detection enabled and the jsondiff.DiffSet is not empty.
//
// The reconciler will emit a Kubernetes event upon completion indicating
// whether the cluster state was successfully corrected or not.
type CorrectClusterDrift struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
	diff          jsondiff.DiffSet
	fieldManager  string
}

func NewCorrectClusterDrift(configFactory *action.ConfigFactory, recorder record.EventRecorder, diff jsondiff.DiffSet, fieldManager string) *CorrectClusterDrift {
	return &CorrectClusterDrift{
		configFactory: configFactory,
		eventRecorder: recorder,
		diff:          diff,
		fieldManager:  fieldManager,
	}
}

func (r *CorrectClusterDrift) Reconcile(ctx context.Context, req *Request) error {
	if req.Object.GetDriftDetection().GetMode() != v2.DriftDetectionEnabled || len(r.diff) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, req.Object.GetTimeout().Duration)
	defer cancel()

	// Update condition to reflect the current status.
	conditions.MarkUnknown(req.Object, meta.ReadyCondition, meta.ProgressingReason, "correcting cluster drift")

	changeSet, err := action.ApplyDiff(ctx, r.configFactory.Build(nil), r.diff, r.fieldManager)
	r.report(req.Object, changeSet, err)
	return nil
}

func (r *CorrectClusterDrift) report(obj *v2.HelmRelease, changeSet *ssa.ChangeSet, err error) {
	cur := obj.Status.History.Latest()

	switch {
	case err != nil:
		var sb strings.Builder
		sb.WriteString("Failed to ")
		if changeSet != nil && len(changeSet.Entries) > 0 {
			sb.WriteString("partially ")
		}
		sb.WriteString("correct cluster state of release ")
		sb.WriteString(cur.FullReleaseName())
		sb.WriteString(":\n")
		if agErr, ok := err.(apierrutil.Aggregate); ok {
			for i := range agErr.Errors() {
				if i > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(agErr.Errors()[i].Error())
			}
		} else {
			sb.WriteString(err.Error())
		}

		if changeSet != nil && len(changeSet.Entries) > 0 {
			sb.WriteString("\n\n")
			sb.WriteString("Successful corrections:\n")
			sb.WriteString(changeSet.String())
		}

		r.eventRecorder.AnnotatedEventf(obj, eventMeta(cur.ChartVersion, cur.ConfigDigest), corev1.EventTypeWarning,
			"DriftCorrectionFailed", sb.String())
	case changeSet != nil && len(changeSet.Entries) > 0:
		r.eventRecorder.AnnotatedEventf(obj, eventMeta(cur.ChartVersion, cur.ConfigDigest), corev1.EventTypeNormal,
			"DriftCorrected", "Cluster state of release %s has been corrected:\n%s",
			obj.Status.History.Latest().FullReleaseName(), changeSet.String())
	}
}

func (r *CorrectClusterDrift) Name() string {
	return "correct cluster drift"
}

func (r *CorrectClusterDrift) Type() ReconcilerType {
	return ReconcilerTypeDriftCorrection
}
