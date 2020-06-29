/*
Copyright 2020 The Flux CD contributors.

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

package controllers

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sourcev1 "github.com/fluxcd/source-controller/api/v1alpha1"

	helmreleasev2 "github.com/fluxcd/helm-controller/api/v2alpha1"
)

// HelmChartWatcher watches HelmChart objects for revision changes and
// triggers a sync for all the HelmReleases that reference a changed source.
type HelmChartWatcher struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=source.fluxcd.io,resources=helmcharts,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=helmcharts/status,verbs=get

func (r *HelmChartWatcher) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var chart sourcev1.HelmChart
	if err := r.Get(ctx, req.NamespacedName, &chart); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log := r.Log.WithValues(strings.ToLower(chart.Kind), req.NamespacedName)
	log.Info("new artifact detected", "revision", chart.GetArtifact().Revision)

	// Get the list of HelmReleases that are using this HelmChart.
	var list helmreleasev2.HelmReleaseList
	if err := r.List(ctx, &list, client.InNamespace(req.Namespace),
		client.MatchingFields{helmreleasev2.SourceIndexKey: req.Name}); err != nil {
		log.Error(err, "unable to list HelmReleases")
		return ctrl.Result{}, err
	}

	// TODO(hidde): dependency sort

	// Trigger reconciliation for each HelmRelease using this HelmChart.
	for _, hr := range list.Items {
		namespacedName := types.NamespacedName{Namespace: hr.Namespace, Name: hr.Name}
		if err := r.requestReconciliation(ctx, hr); err != nil {
			log.Error(err, "unable to annotate HelmRelease", strings.ToLower(hr.Kind), namespacedName)
			continue
		}
		log.Info("requested immediate reconciliation", strings.ToLower(hr.Kind), namespacedName)
	}

	return ctrl.Result{}, nil
}

func (r *HelmChartWatcher) SetupWithManager(mgr ctrl.Manager) error {
	// Create a HelmRelease index based on the HelmChart name
	err := mgr.GetFieldIndexer().IndexField(context.TODO(), &helmreleasev2.HelmRelease{}, helmreleasev2.SourceIndexKey,
		func(rawObj runtime.Object) []string {
			hr := rawObj.(*helmreleasev2.HelmRelease)
			if hr.Spec.SourceRef.Kind == "HelmChart" {
				return []string{hr.Spec.SourceRef.Name}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&sourcev1.HelmChart{}).
		WithEventFilter(HelmChartRevisionChangePredicate{}).
		Complete(r)
}

// requestReconciliation annotates the given HelmRelease to be reconciled immediately.
func (r *HelmChartWatcher) requestReconciliation(ctx context.Context, hr helmreleasev2.HelmRelease) error {
	firstTry := true
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if !firstTry {
			if err := r.Get(context.TODO(),
				types.NamespacedName{Namespace: hr.Namespace, Name: hr.Name},
				&hr,
			); err != nil {
				return err
			}
		}

		firstTry = false
		if hr.Annotations == nil {
			hr.Annotations = make(map[string]string)
		}
		hr.Annotations[helmreleasev2.ReconcileAtAnnotation] = metav1.Now().String()
		// Prevent strings can't be nil err as API package does not mark APIGroup with omitempty.
		if hr.Spec.SourceRef.APIGroup == nil {
			emptyAPIGroup := ""
			hr.Spec.SourceRef.APIGroup = &emptyAPIGroup
		}
		err = r.Update(ctx, &hr)
		return
	})
}
