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

package controllers

import (
	"context"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/fluxcd/pkg/runtime/acl"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/opencontainers/go-digest"
	_ "github.com/opencontainers/go-digest/blake3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func (r *HelmReleaseReconciler) reconcileChart(ctx context.Context, hr *v2.HelmRelease) (*sourcev1b2.HelmChart, error) {
	chartName := types.NamespacedName{
		Namespace: hr.Spec.Chart.GetNamespace(hr.Namespace),
		Name:      hr.GetHelmChartName(),
	}

	if r.NoCrossNamespaceRef && chartName.Namespace != hr.Namespace {
		return nil, acl.AccessDeniedError(fmt.Sprintf("can't access '%s/%s', cross-namespace references have been blocked",
			hr.Spec.Chart.Spec.SourceRef.Kind, types.NamespacedName{
				Namespace: hr.Spec.Chart.Spec.SourceRef.Namespace,
				Name:      hr.Spec.Chart.Spec.SourceRef.Name,
			}))
	}

	// Garbage collect the previous HelmChart if the namespace named changed.
	if hr.Status.HelmChart != "" && hr.Status.HelmChart != chartName.String() {
		if err := r.deleteHelmChart(ctx, hr); err != nil {
			return nil, err
		}
	}

	// Continue with the reconciliation of the current template.
	var helmChart sourcev1b2.HelmChart
	err := r.Client.Get(ctx, chartName, &helmChart)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	hc := buildHelmChartFromTemplate(hr)
	switch {
	case apierrors.IsNotFound(err):
		if err = r.Client.Create(ctx, hc); err != nil {
			return nil, err
		}
		hr.Status.HelmChart = chartName.String()
		return hc, nil
	case helmChartRequiresUpdate(hr, &helmChart):
		ctrl.LoggerFrom(ctx).Info("chart diverged from template", strings.ToLower(sourcev1b2.HelmChartKind), chartName.String())
		helmChart.Spec = hc.Spec
		helmChart.Labels = hc.Labels
		helmChart.Annotations = hc.Annotations

		if err = r.Client.Update(ctx, &helmChart); err != nil {
			return nil, err
		}
		hr.Status.HelmChart = chartName.String()
	}
	return &helmChart, nil
}

// getHelmChart retrieves the v1beta2.HelmChart for the given
// v2beta1.HelmRelease using the name that is advertised in the status
// object. It returns the v1beta2.HelmChart, or an error.
func (r *HelmReleaseReconciler) getHelmChart(ctx context.Context, hr *v2.HelmRelease) (*sourcev1b2.HelmChart, error) {
	namespace, name := hr.Status.GetHelmChart()
	hc := &sourcev1b2.HelmChart{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, hc); err != nil {
		return nil, err
	}
	return hc, nil
}

// loadHelmChart attempts to download the artifact from the provided source,
// loads it into a chart.Chart, and removes the downloaded artifact.
// It returns the loaded chart.Chart on success, or an error.
func (r *HelmReleaseReconciler) loadHelmChart(source *sourcev1b2.HelmChart) (*chart.Chart, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("%s-%s-*.tgz", source.GetNamespace(), source.GetName()))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())

	artifactURL := source.GetArtifact().URL
	if hostname := os.Getenv("SOURCE_CONTROLLER_LOCALHOST"); hostname != "" {
		u, err := url.Parse(artifactURL)
		if err != nil {
			return nil, err
		}
		u.Host = hostname
		artifactURL = u.String()
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, artifactURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact, error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifact '%s' download failed (status code: %s)", source.GetArtifact().URL, resp.Status)
	}

	// verify checksum matches origin
	if err := r.copyAndVerifyArtifact(source.GetArtifact(), resp.Body, f); err != nil {
		return nil, err
	}

	return loader.Load(f.Name())
}

func (r *HelmReleaseReconciler) copyAndVerifyArtifact(artifact *sourcev1.Artifact, reader io.Reader, writer io.Writer) error {
	dig, err := digest.Parse(artifact.Digest)
	if err != nil {
		return fmt.Errorf("failed to verify artifact: %w", err)
	}

	// Verify the downloaded artifact against the advertised digest.
	verifier := dig.Verifier()
	mw := io.MultiWriter(verifier, writer)
	if _, err := io.Copy(mw, reader); err != nil {
		return err
	}

	if !verifier.Verified() {
		return fmt.Errorf("failed to verify artifact: computed digest doesn't match advertised '%s'", dig)
	}
	return nil
}

// deleteHelmChart deletes the v1beta2.HelmChart of the v2beta1.HelmRelease.
func (r *HelmReleaseReconciler) deleteHelmChart(ctx context.Context, hr *v2.HelmRelease) error {
	if hr.Status.HelmChart == "" {
		return nil
	}
	var hc sourcev1b2.HelmChart
	chartNS, chartName := hr.Status.GetHelmChart()
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: chartNS, Name: chartName}, &hc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			hr.Status.HelmChart = ""
			return nil
		}
		err = fmt.Errorf("failed to delete HelmChart '%s': %w", hr.Status.HelmChart, err)
		return err
	}
	if err = r.Client.Delete(ctx, &hc); err != nil {
		err = fmt.Errorf("failed to delete HelmChart '%s': %w", hr.Status.HelmChart, err)
		return err
	}
	// Truncate the chart reference in the status object.
	hr.Status.HelmChart = ""
	return nil
}

// buildHelmChartFromTemplate builds a v1beta2.HelmChart from the
// v2beta1.HelmChartTemplate of the given v2beta1.HelmRelease.
func buildHelmChartFromTemplate(hr *v2.HelmRelease) *sourcev1b2.HelmChart {
	template := hr.Spec.Chart
	result := &sourcev1b2.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hr.GetHelmChartName(),
			Namespace: hr.Spec.Chart.GetNamespace(hr.Namespace),
		},
		Spec: sourcev1b2.HelmChartSpec{
			Chart:   template.Spec.Chart,
			Version: template.Spec.Version,
			SourceRef: sourcev1b2.LocalHelmChartSourceReference{
				Name: template.Spec.SourceRef.Name,
				Kind: template.Spec.SourceRef.Kind,
			},
			Interval:          template.GetInterval(hr.Spec.Interval),
			ReconcileStrategy: template.Spec.ReconcileStrategy,
			ValuesFiles:       template.Spec.ValuesFiles,
			ValuesFile:        template.Spec.ValuesFile,
			Verify:            templateVerificationToSourceVerification(template.Spec.Verify),
		},
	}
	if hr.Spec.Chart.ObjectMeta != nil {
		result.ObjectMeta.Labels = hr.Spec.Chart.ObjectMeta.Labels
		result.ObjectMeta.Annotations = hr.Spec.Chart.ObjectMeta.Annotations
	}
	return result
}

// helmChartRequiresUpdate compares the v2beta1.HelmChartTemplate of the
// v2beta1.HelmRelease to the given v1beta2.HelmChart to determine if an
// update is required.
func helmChartRequiresUpdate(hr *v2.HelmRelease, chart *sourcev1b2.HelmChart) bool {
	template := hr.Spec.Chart
	switch {
	case template.Spec.Chart != chart.Spec.Chart:
		return true
	// TODO(hidde): remove emptiness checks on next MINOR version
	case template.Spec.Version == "" && chart.Spec.Version != "*",
		template.Spec.Version != "" && template.Spec.Version != chart.Spec.Version:
		return true
	case template.Spec.SourceRef.Name != chart.Spec.SourceRef.Name:
		return true
	case template.Spec.SourceRef.Kind != chart.Spec.SourceRef.Kind:
		return true
	case template.GetInterval(hr.Spec.Interval) != chart.Spec.Interval:
		return true
	case template.Spec.ReconcileStrategy != chart.Spec.ReconcileStrategy:
		return true
	case !reflect.DeepEqual(template.Spec.ValuesFiles, chart.Spec.ValuesFiles):
		return true
	case template.Spec.ValuesFile != chart.Spec.ValuesFile:
		return true
	case template.ObjectMeta != nil && !apiequality.Semantic.DeepEqual(template.ObjectMeta.Annotations, chart.Annotations):
		return true
	case template.ObjectMeta != nil && !apiequality.Semantic.DeepEqual(template.ObjectMeta.Labels, chart.Labels):
		return true
	case !reflect.DeepEqual(templateVerificationToSourceVerification(template.Spec.Verify), chart.Spec.Verify):
		return true
	default:
		return false
	}
}

// templateVerificationToSourceVerification converts the HelmChartTemplateVerification to the OCIRepositoryVerification.
func templateVerificationToSourceVerification(template *v2.HelmChartTemplateVerification) *sourcev1b2.OCIRepositoryVerification {
	if template == nil {
		return nil
	}

	return &sourcev1b2.OCIRepositoryVerification{
		Provider:  template.Provider,
		SecretRef: template.SecretRef,
	}
}
