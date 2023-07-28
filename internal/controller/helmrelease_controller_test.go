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

package controller

import (
	"context"
	"errors"
	"k8s.io/client-go/rest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/acl"
	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/reconcile"
)

func TestHelmReleaseReconciler_reconcileChartTemplate(t *testing.T) {
	t.Run("attempts to reconcile chart template", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseReconciler{
			Client:        fake.NewClientBuilder().WithScheme(NewTestScheme()).Build(),
			EventRecorder: record.NewFakeRecorder(32),
		}

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "default",
			},
		}

		// We do not care about the result of the reconcile, only that it was attempted.
		err := r.reconcileChartTemplate(context.TODO(), obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to run server-side apply"))
	})
}

func TestHelmReleaseReconciler_reconcileUninstall(t *testing.T) {
	t.Run("attempts to uninstall release", func(t *testing.T) {
		g := NewWithT(t)

		getter := kube.NewMemoryRESTClientGetter(testEnv.GetConfig())

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "default",
			},
		}

		// We do not care about the result of the uninstall, only that it was attempted.
		err := (&HelmReleaseReconciler{}).reconcileUninstall(context.TODO(), getter, obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, reconcile.ErrNoCurrent)).To(BeTrue())
	})

	t.Run("error on empty storage namespace", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				StorageNamespace: "",
			},
		}

		err := (&HelmReleaseReconciler{}).reconcileUninstall(context.TODO(), nil, obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(conditions.IsFalse(obj, meta.ReadyCondition)).To(BeTrue())
		g.Expect(conditions.GetReason(obj, meta.ReadyCondition)).To(Equal("ConfigFactoryErr"))
		g.Expect(conditions.GetMessage(obj, meta.ReadyCondition)).To(ContainSubstring("no namespace provided"))
		g.Expect(obj.GetConditions()).To(HaveLen(1))
	})
}

func TestHelmReleaseReconciler_checkDependencies(t *testing.T) {
	tests := []struct {
		name    string
		obj     *v2.HelmRelease
		objects []client.Object
		expect  func(g *WithT, err error)
	}{
		{
			name: "all dependencies ready",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
						{
							Name:      "dependency-2",
							Namespace: "some-other-namespace",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
						Name:       "dependency-2",
						Namespace:  "some-other-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 2,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).ToNot(HaveOccurred())
			},
		},
		{
			name: "error on dependency with ObservedGeneration < Generation",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is not ready"))
			},
		},
		{
			name: "error on dependency with ObservedGeneration = Generation and ReadyCondition = False",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
						Conditions: []metav1.Condition{
							{Type: meta.ReadyCondition, Status: metav1.ConditionFalse},
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is not ready"))
			},
		},
		{
			name: "error on dependency without conditions",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			objects: []client.Object{
				&v2.HelmRelease{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
						Name:       "dependency-1",
						Namespace:  "some-namespace",
					},
					Status: v2.HelmReleaseStatus{
						ObservedGeneration: 1,
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is not ready"))
			},
		},
		{
			name: "error on missing dependency",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dependant",
					Namespace: "some-namespace",
				},
				Spec: v2.HelmReleaseSpec{
					DependsOn: []meta.NamespacedObjectReference{
						{
							Name: "dependency-1",
						},
					},
				},
			},
			expect: func(g *WithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithScheme(NewTestScheme())
			if len(tt.objects) > 0 {
				c.WithObjects(tt.objects...)
			}

			r := &HelmReleaseReconciler{
				Client: c.Build(),
			}

			err := r.checkDependencies(context.TODO(), tt.obj)
			tt.expect(g, err)
		})
	}
}

func TestHelmReleaseReconciler_buildRESTClientGetter(t *testing.T) {
	const (
		namespace = "some-namespace"
		kubeCfg   = `apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: https://1.2.3.4
  name: development
contexts:
- context:
    cluster: development
    namespace: frontend
    user: developer
  name: dev-frontend
current-context: dev-frontend
preferences: {}
users:
- name: developer
  user:
    password: some-password
    username: exp`
	)

	tests := []struct {
		name      string
		env       map[string]string
		getConfig func() (*rest.Config, error)
		spec      v2.HelmReleaseSpec
		secret    *corev1.Secret
		want      genericclioptions.RESTClientGetter
		wantErr   string
	}{
		{
			name: "builds in-cluster RESTClientGetter for HelmRelease",
			getConfig: func() (*rest.Config, error) {
				return clientcmd.RESTConfigFromKubeConfig([]byte(kubeCfg))
			},
			spec: v2.HelmReleaseSpec{},
			want: &kube.MemoryRESTClientGetter{},
		},
		{
			name: "returns error when in-cluster GetClusterConfig fails",
			getConfig: func() (*rest.Config, error) {
				return nil, errors.New("some-error")
			},
			wantErr: "some-error",
		},
		{
			name: "builds RESTClientGetter from HelmRelease with KubeConfig",
			spec: v2.HelmReleaseSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: "kubeconfig",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeconfig",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					kube.DefaultKubeConfigSecretKey: []byte(kubeCfg),
				},
			},
			want: &kube.MemoryRESTClientGetter{},
		},
		{
			name: "error on missing KubeConfig secret",
			spec: v2.HelmReleaseSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: "kubeconfig",
					},
				},
			},
			wantErr: "could not get KubeConfig secret",
		},
		{
			name: "error on invalid KubeConfig secret",
			spec: v2.HelmReleaseSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: "kubeconfig",
						Key:  "invalid-key",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeconfig",
					Namespace: namespace,
				},
			},
			wantErr: "does not contain a 'invalid-key' key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			c := fake.NewClientBuilder()
			if tt.secret != nil {
				c.WithObjects(tt.secret)
			}

			r := &HelmReleaseReconciler{
				Client:           c.Build(),
				GetClusterConfig: tt.getConfig,
			}

			getter, err := r.buildRESTClientGetter(context.Background(), &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-name",
					Namespace: namespace,
				},
				Spec: tt.spec,
			})
			if len(tt.wantErr) > 0 {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(getter).To(BeAssignableToTypeOf(tt.want))
			}
		})
	}
}

func TestHelmReleaseReconciler_getHelmChart(t *testing.T) {
	g := NewWithT(t)

	chart := &sourcev1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "some-namespace",
			Name:      "some-chart-name",
		},
	}

	tests := []struct {
		name            string
		rel             *v2.HelmRelease
		chart           *sourcev1.HelmChart
		expectChart     bool
		wantErr         bool
		disallowCrossNS bool
	}{
		{
			name: "retrieves HelmChart object from Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       chart,
			expectChart: true,
		},
		{
			name: "no HelmChart found",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       nil,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "no HelmChart in Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "",
				},
			},
			chart:       chart,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "ACL disallows cross namespace",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:           chart,
			expectChart:     false,
			wantErr:         true,
			disallowCrossNS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder()
			c.WithScheme(NewTestScheme())
			if tt.chart != nil {
				c.WithObjects(tt.chart)
			}

			r := &HelmReleaseReconciler{
				Client:        c.Build(),
				EventRecorder: record.NewFakeRecorder(32),
			}

			curAllow := acl.AllowCrossNamespaceRef
			acl.AllowCrossNamespaceRef = !tt.disallowCrossNS
			t.Cleanup(func() { acl.AllowCrossNamespaceRef = !curAllow })

			got, err := r.getHelmChart(context.TODO(), tt.rel)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			expect := g.Expect(got.ObjectMeta)
			if tt.expectChart {
				expect.To(BeEquivalentTo(tt.chart.ObjectMeta))
			} else {
				expect.To(BeNil())
			}
		})
	}
}

func TestValuesReferenceValidation(t *testing.T) {
	tests := []struct {
		name       string
		references []v2.ValuesReference
		wantErr    bool
	}{
		{
			name: "valid ValuesKey",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "any-key_na.me",
				},
			},
			wantErr: false,
		},
		{
			name: "valid ValuesKey: empty",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "",
				},
			},
			wantErr: false,
		},
		{
			name: "valid ValuesKey: long",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: strings.Repeat("a", 253),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid ValuesKey",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "a($&^%b",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid ValuesKey: too long",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: strings.Repeat("a", 254),
				},
			},
			wantErr: true,
		},
		{
			name: "valid target path: empty",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: "",
				},
			},
			wantErr: false,
		},
		{
			name: "valid target path",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: "list_with.nested-values.and.index[0]",
				},
			},
			wantErr: false,
		},
		{
			name: "valid target path: long",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: strings.Repeat("a", 250),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid target path: too long",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: strings.Repeat("a", 251),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid target path: opened index",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "a[",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid target path: incorrect index syntax",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "a]0[",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var values *apiextensionsv1.JSON
			v, _ := yaml.YAMLToJSON([]byte("values"))
			values = &apiextensionsv1.JSON{Raw: v}

			hr := v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: 5 * time.Minute},
					Chart: v2.HelmChartTemplate{
						Spec: v2.HelmChartTemplateSpec{
							Chart: "mychart",
							SourceRef: v2.CrossNamespaceObjectReference{
								Name: "something",
							},
						},
					},
					ValuesFrom: tt.references,
					Values:     values,
				},
			}

			err := testEnv.Create(context.TODO(), &hr, client.DryRunAll)
			if (err != nil) != tt.wantErr {
				t.Errorf("composeValues() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
