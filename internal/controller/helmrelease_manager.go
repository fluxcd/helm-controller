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
	"time"

	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	intpredicates "github.com/fluxcd/helm-controller/internal/predicates"
)

type HelmReleaseReconcilerOptions struct {
	HTTPRetry                 int
	DependencyRequeueInterval time.Duration
	RateLimiter               workqueue.TypedRateLimiter[reconcile.Request]
	WatchConfigsPredicate     predicate.Predicate
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

	r.requeueDependency = opts.DependencyRequeueInterval
	r.artifactFetchRetries = opts.HTTPRetry

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&sourcev1.HelmChart{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForHelmChartChange),
			builder.WithPredicates(intpredicates.SourceRevisionChangePredicate{}),
		).
		Watches(
			&sourcev1.OCIRepository{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForOCIRepositoryChange),
			builder.WithPredicates(intpredicates.SourceRevisionChangePredicate{}),
		).
		WatchesMetadata(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForConfigDependency(indexConfigMap)),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, opts.WatchConfigsPredicate),
		).
		WatchesMetadata(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForConfigDependency(indexSecret)),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, opts.WatchConfigsPredicate),
		).
		WithOptions(controller.Options{
			RateLimiter: opts.RateLimiter,
		}).
		Complete(r)
}
