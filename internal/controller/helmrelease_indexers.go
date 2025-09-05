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

	"github.com/fluxcd/pkg/runtime/conditions"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// requestsForHelmChartChange enqueues requests for watched HelmCharts
// according to the specified index.
func (r *HelmReleaseReconciler) requestsForHelmChartChange(ctx context.Context, o client.Object) []reconcile.Request {
	hc, ok := o.(*sourcev1.HelmChart)
	if !ok {
		err := fmt.Errorf("expected a HelmChart, got %T", o)
		ctrl.LoggerFrom(ctx).Error(err, "failed to get requests for HelmChart change")
		return nil
	}
	// If we do not have an artifact, we have no requests to make
	if hc.GetArtifact() == nil {
		return nil
	}

	var list v2.HelmReleaseList
	if err := r.List(ctx, &list, client.MatchingFields{
		v2.SourceIndexKey: sourcev1.HelmChartKind + "/" + client.ObjectKeyFromObject(hc).String(),
	}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to list HelmReleases for HelmChart change")
		return nil
	}

	var reqs []reconcile.Request
	for i, hr := range list.Items {
		// If the HelmRelease is ready and the revision of the artifact equals to the
		// last attempted revision, we should not make a request for this HelmRelease
		if conditions.IsReady(&list.Items[i]) && hc.GetArtifact().HasRevision(hr.Status.GetLastAttemptedRevision()) {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

// requestsForOCIRepositoryChange enqueues requests for watched OCIRepositories
// according to the specified index.
func (r *HelmReleaseReconciler) requestsForOCIRepositoryChange(ctx context.Context, o client.Object) []reconcile.Request {
	or, ok := o.(*sourcev1.OCIRepository)
	if !ok {
		err := fmt.Errorf("expected an OCIRepository, got %T", o)
		ctrl.LoggerFrom(ctx).Error(err, "failed to get requests for OCIRepository change")
		return nil
	}
	// If we do not have an artifact, we have no requests to make
	if or.GetArtifact() == nil {
		return nil
	}

	var list v2.HelmReleaseList
	if err := r.List(ctx, &list, client.MatchingFields{
		v2.SourceIndexKey: sourcev1.OCIRepositoryKind + "/" + client.ObjectKeyFromObject(or).String(),
	}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to list HelmReleases for OCIRepository change")
		return nil
	}

	var reqs []reconcile.Request
	for i, hr := range list.Items {
		// If the HelmRelease is ready and the digest of the artifact equals to the
		// last attempted revision digest, we should not make a request for this HelmRelease,
		// likewise if we cannot retrieve the artifact digest.
		digest := extractDigest(or.GetArtifact().Revision)
		if digest == "" {
			ctrl.LoggerFrom(ctx).Error(fmt.Errorf("wrong digest for %T", or), "failed to get requests for OCIRepository change")
			continue
		}

		if digest == hr.Status.LastAttemptedRevisionDigest {
			continue
		}

		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

// requestsForExternalArtifactChange enqueues requests for watched ExternalArtifacts
// according to the specified index.
func (r *HelmReleaseReconciler) requestsForExternalArtifactChange(ctx context.Context, o client.Object) []reconcile.Request {
	or, ok := o.(*sourcev1.ExternalArtifact)
	if !ok {
		err := fmt.Errorf("expected an ExternalArtifact, got %T", o)
		ctrl.LoggerFrom(ctx).Error(err, "failed to get requests for ExternalArtifact change")
		return nil
	}
	// If we do not have an artifact, we have no requests to make
	if or.GetArtifact() == nil {
		return nil
	}

	var list v2.HelmReleaseList
	if err := r.List(ctx, &list, client.MatchingFields{
		v2.SourceIndexKey: sourcev1.ExternalArtifactKind + "/" + client.ObjectKeyFromObject(or).String(),
	}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to list HelmReleases for ExternalArtifact change")
		return nil
	}

	var reqs []reconcile.Request
	for i, hr := range list.Items {
		// If the HelmRelease is ready and the digest of the artifact equals to the
		// last attempted revision digest, we should not make a request for this HelmRelease,
		// likewise if we cannot retrieve the artifact digest.
		digest := extractDigest(or.GetArtifact().Revision)
		if digest == "" {
			ctrl.LoggerFrom(ctx).Error(fmt.Errorf("wrong digest for %T", or), "failed to get requests for ExternalArtifact change")
			continue
		}

		if digest == hr.Status.LastAttemptedRevisionDigest {
			continue
		}

		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

// requestsForConfigDependency enqueues requests for watched ConfigMaps or Secrets
// according to the specified index.
func (r *HelmReleaseReconciler) requestsForConfigDependency(
	index string) func(ctx context.Context, o client.Object) []reconcile.Request {

	return func(ctx context.Context, o client.Object) []reconcile.Request {
		// List HelmReleases that have a dependency on the ConfigMap or Secret.
		var list v2.HelmReleaseList
		if err := r.List(ctx, &list, client.MatchingFields{
			index: client.ObjectKeyFromObject(o).String(),
		}); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "failed to list HelmReleases for config dependency change",
				"index", index, "objectRef", map[string]string{
					"name":      o.GetName(),
					"namespace": o.GetNamespace(),
				})
			return nil
		}

		// Enqueue requests for each HelmRelease in the list.
		reqs := make([]reconcile.Request, 0, len(list.Items))
		for i := range list.Items {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
		return reqs
	}
}
