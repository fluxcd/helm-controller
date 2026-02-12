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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runtimeCtrl "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	intpredicates "github.com/fluxcd/helm-controller/internal/predicates"
)

type HelmReleaseReconcilerOptions struct {
	RateLimiter                workqueue.TypedRateLimiter[reconcile.Request]
	WatchConfigs               bool
	WatchConfigsPredicate      predicate.Predicate
	WatchExternalArtifacts     bool
	CancelHealthCheckOnRequeue bool
}

func (r *HelmReleaseReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	const (
		indexConfigMap = ".metadata.configMap"
		indexSecret    = ".metadata.secret"
	)

	// Index the HelmRelease by the Source reference they point to.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v2.HelmRelease{}, v2.SourceIndexKey,
		func(o client.Object) []string {
			obj := o.(*v2.HelmRelease)
			var kind, name, namespace string
			switch {
			case obj.HasChartRef() && !obj.HasChartTemplate():
				kind = obj.Spec.ChartRef.Kind
				name = obj.Spec.ChartRef.Name
				namespace = obj.Spec.ChartRef.Namespace
				if namespace == "" {
					namespace = obj.GetNamespace()
				}
			case !obj.HasChartRef() && obj.HasChartTemplate():
				kind = sourcev1.HelmChartKind
				name = obj.GetHelmChartName()
				namespace = obj.Spec.Chart.GetNamespace(obj.GetNamespace())
			default:
				return nil
			}
			return []string{fmt.Sprintf("%s/%s/%s", kind, namespace, name)}
		},
	); err != nil {
		return err
	}

	// Index the HelmRelease by the ConfigMap references they point to.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v2.HelmRelease{}, indexConfigMap,
		func(o client.Object) []string {
			obj := o.(*v2.HelmRelease)
			namespace := obj.GetNamespace()
			var keys []string
			if kc := obj.Spec.KubeConfig; kc != nil && kc.ConfigMapRef != nil {
				keys = append(keys, fmt.Sprintf("%s/%s", namespace, kc.ConfigMapRef.Name))
			}
			for _, ref := range obj.Spec.ValuesFrom {
				if ref.Kind == "ConfigMap" {
					keys = append(keys, fmt.Sprintf("%s/%s", namespace, ref.Name))
				}
			}
			return keys
		},
	); err != nil {
		return err
	}

	// Index the HelmRelease by the Secret references they point to.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v2.HelmRelease{}, indexSecret,
		func(o client.Object) []string {
			obj := o.(*v2.HelmRelease)
			namespace := obj.GetNamespace()
			var keys []string
			if kc := obj.Spec.KubeConfig; kc != nil && kc.SecretRef != nil {
				keys = append(keys, fmt.Sprintf("%s/%s", namespace, kc.SecretRef.Name))
			}
			for _, ref := range obj.Spec.ValuesFrom {
				if ref.Kind == "Secret" {
					keys = append(keys, fmt.Sprintf("%s/%s", namespace, ref.Name))
				}
			}
			return keys
		},
	); err != nil {
		return err
	}

	var blder *builder.Builder
	var toComplete reconcile.TypedReconciler[reconcile.Request]
	var enqueueRequestsFromMapFunc func(objKind string, fn handler.MapFunc) handler.EventHandler

	hrPredicate := predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicates.ReconcileRequestedPredicate{},
	)

	if !opts.CancelHealthCheckOnRequeue {
		toComplete = r
		enqueueRequestsFromMapFunc = func(objKind string, fn handler.MapFunc) handler.EventHandler {
			return handler.EnqueueRequestsFromMapFunc(fn)
		}
		blder = ctrl.NewControllerManagedBy(mgr).
			For(&v2.HelmRelease{}, builder.WithPredicates(hrPredicate))
	} else {
		wr := runtimeCtrl.WrapReconciler(r)
		toComplete = wr
		enqueueRequestsFromMapFunc = wr.EnqueueRequestsFromMapFunc
		blder = runtimeCtrl.NewControllerManagedBy(mgr, wr).
			For(&v2.HelmRelease{}, hrPredicate).Builder
	}

	blder.
		Watches(
			&sourcev1.HelmChart{},
			enqueueRequestsFromMapFunc(sourcev1.HelmChartKind, r.requestsForHelmChartChange),
			builder.WithPredicates(intpredicates.SourceRevisionChangePredicate{}),
		).
		Watches(
			&sourcev1.OCIRepository{},
			enqueueRequestsFromMapFunc(sourcev1.OCIRepositoryKind, r.requestsForOCIRepositoryChange),
			builder.WithPredicates(intpredicates.SourceRevisionChangePredicate{}),
		)

	if opts.WatchConfigs {
		blder = blder.
			WatchesMetadata(
				&corev1.ConfigMap{},
				enqueueRequestsFromMapFunc("ConfigMap", r.requestsForConfigDependency(indexConfigMap)),
				builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, opts.WatchConfigsPredicate),
			).
			WatchesMetadata(
				&corev1.Secret{},
				enqueueRequestsFromMapFunc("Secret", r.requestsForConfigDependency(indexSecret)),
				builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, opts.WatchConfigsPredicate),
			)
	}

	if opts.WatchExternalArtifacts {
		blder = blder.Watches(
			&sourcev1.ExternalArtifact{},
			enqueueRequestsFromMapFunc(sourcev1.ExternalArtifactKind, r.requestsForExternalArtifactChange),
			builder.WithPredicates(intpredicates.SourceRevisionChangePredicate{}),
		)
	}

	return blder.WithOptions(controller.Options{RateLimiter: opts.RateLimiter}).Complete(toComplete)
}
