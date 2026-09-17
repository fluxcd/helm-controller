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

package action

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
	helmaction "helm.sh/helm/v4/pkg/action"
	helmchartcommon "helm.sh/helm/v4/pkg/chart/common"
	helmchart "helm.sh/helm/v4/pkg/chart/v2"
	helmkube "helm.sh/helm/v4/pkg/kube"
	helmkubefake "helm.sh/helm/v4/pkg/kube/fake"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/kube"
)

// CRD + non-CRD content as they would appear in a chart's crds/ directory,
// mirroring the layout in envoyproxy/gateway's gateway-helm chart that
// surfaced fluxcd/helm-controller#1486.
const (
	testCRDYAML = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: foos.example.com
spec:
  group: example.com
  names:
    kind: Foo
    plural: foos
  scope: Namespaced
`
	testVAPYAML = `apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: safe-upgrades.example.com
spec:
  failurePolicy: Fail
`
)

// stubKubeClient is a helmkube.Interface that parses YAML into resource.Info
// without touching a real apiserver, and records the calls applyCRDs makes
// against it. Methods we don't care about are inherited from PrintingKubeClient.
type stubKubeClient struct {
	*helmkubefake.PrintingKubeClient

	// client is attached to the Infos returned by Build, mirroring how the
	// real Helm kube client populates resource.Info.Client.
	client resource.RESTClient

	createCalls []helmkube.ResourceList
	updateCalls []struct {
		Original, Target helmkube.ResourceList
	}
}

func (c *stubKubeClient) Build(r io.Reader, _ bool) (helmkube.ResourceList, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(data, u); err != nil {
		return nil, err
	}
	gvk := u.GroupVersionKind()
	scope := apimeta.RESTScopeNamespace
	if u.GetNamespace() == "" {
		scope = apimeta.RESTScopeRoot
	}
	info := &resource.Info{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
		Object:    u,
		Client:    c.client,
		Mapping: &apimeta.RESTMapping{
			GroupVersionKind: gvk,
			Resource: schema.GroupVersionResource{
				Group:    gvk.Group,
				Version:  gvk.Version,
				Resource: strings.ToLower(gvk.Kind) + "s",
			},
			Scope: scope,
		},
	}
	return helmkube.ResourceList{info}, nil
}

func (c *stubKubeClient) Create(rl helmkube.ResourceList, _ ...helmkube.ClientCreateOption) (*helmkube.Result, error) {
	cp := make(helmkube.ResourceList, len(rl))
	copy(cp, rl)
	c.createCalls = append(c.createCalls, cp)
	return &helmkube.Result{Created: cp}, nil
}

func (c *stubKubeClient) Update(original, target helmkube.ResourceList, _ ...helmkube.ClientUpdateOption) (*helmkube.Result, error) {
	c.updateCalls = append(c.updateCalls, struct {
		Original, Target helmkube.ResourceList
	}{original, target})
	return &helmkube.Result{Updated: target}, nil
}

// testChart returns a chart whose crds/ directory contains one CRD and one
// non-CRD (a ValidatingAdmissionPolicy), matching the layout that produced
// envoyproxy/gateway#9015 / fluxcd/helm-controller#1486.
func testChart() *helmchart.Chart {
	return &helmchart.Chart{
		Metadata: &helmchart.Metadata{
			Name:       "test",
			Version:    "0.1.0",
			APIVersion: "v2",
		},
		Files: []*helmchartcommon.File{
			{Name: "crds/crd.yaml", Data: []byte(testCRDYAML)},
			{Name: "crds/vap.yaml", Data: []byte(testVAPYAML)},
		},
	}
}

// fakeAPIServer records every GET path it sees and responds 404 by default, so
// that any apiextensions client.Get() call surfaces as IsNotFound but is also
// captured for assertion. CRDs registered through AddCRD respond 200 instead.
type fakeAPIServer struct {
	server *httptest.Server

	mu           sync.Mutex
	getPaths     []string
	existingCRDs map[string]bool
}

func newFakeAPIServer(t *testing.T) *fakeAPIServer {
	t.Helper()
	f := &fakeAPIServer{existingCRDs: make(map[string]bool)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			name := path.Base(r.URL.Path)
			f.mu.Lock()
			f.getPaths = append(f.getPaths, r.URL.Path)
			exists := f.existingCRDs[name]
			f.mu.Unlock()
			if exists {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w,
					`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":%q}}`,
					name)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// AddCRD marks the named CustomResourceDefinition as existing, making GET
// requests for it return 200 instead of 404.
func (f *fakeAPIServer) AddCRD(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existingCRDs[name] = true
}

func (f *fakeAPIServer) GetPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.getPaths))
	copy(out, f.getPaths)
	return out
}

// expectedWarningMessage is the exact message body the filter must emit, so
// accidental rewordings become test failures.
const expectedWarningMessage = "warning: ignoring non-CRD resources found in the chart's crds/ directory; " +
	"the crds/ directory is reserved for CustomResourceDefinition objects only, " +
	"please move these resources to templates/ in the chart. if the chart is " +
	"public, please open an issue in the upstream repository to have this fixed"

// expectedNonCRDJSON is the JSON shape the controller-runtime logger renders
// for the nonCRDs structured attribute on the warning record.
const expectedNonCRDJSON = `"nonCRDs":["admissionregistration.k8s.io/ValidatingAdmissionPolicy/safe-upgrades.example.com"]`

// newTestCfg wires a Configuration that exercises the production slog plumbing
// (NewDebugLogBuffer, identical to internal/reconcile/install.go and upgrade.go)
// but with a stub KubeClient and an httptest-backed RESTClientGetter so the
// test never needs a live apiserver. The returned ctx carries a JSON-rendering
// logr sink so callers can assert on the controller-runtime log output that
// applyCRDs writes via ctrl.LoggerFrom(ctx).
func newTestCfg(t *testing.T) (context.Context, *helmaction.Configuration, *stubKubeClient, *fakeAPIServer, *bytes.Buffer) {
	t.Helper()
	api := newFakeAPIServer(t)
	getter := kube.NewMemoryRESTClientGetter(&rest.Config{Host: api.server.URL})

	// Build a REST client for the CRD API group the same way the real Helm
	// kube client does, so that resource.Info.Get() works against the fake
	// apiserver.
	restCfg, err := getter.ToRESTConfig()
	if err != nil {
		t.Fatalf("failed to get REST config: %v", err)
	}
	restCfg = rest.CopyConfig(restCfg)
	restCfg.ContentConfig = resource.UnstructuredPlusDefaultContentConfig()
	restCfg.GroupVersion = &schema.GroupVersion{Group: "apiextensions.k8s.io", Version: "v1"}
	restCfg.APIPath = "/apis"
	restClient, err := rest.RESTClientFor(restCfg)
	if err != nil {
		t.Fatalf("failed to create REST client: %v", err)
	}

	kc := &stubKubeClient{
		client: restClient,
		PrintingKubeClient: &helmkubefake.PrintingKubeClient{
			Out:       io.Discard,
			LogOutput: io.Discard,
		},
	}
	ctxLog := &bytes.Buffer{}
	ctxLogger := funcr.NewJSON(func(s string) { ctxLog.WriteString(s + "\n") }, funcr.Options{})
	ctx := log.IntoContext(context.Background(), ctxLogger)
	logBuf := NewDebugLogBuffer(ctx)
	cfg := &helmaction.Configuration{
		KubeClient:       kc,
		RESTClientGetter: getter,
	}
	cfg.SetLogger(logBuf)
	return ctx, cfg, kc, api, ctxLog
}

// The non-CRD filter and warning apply for both policies: the Helm crds/
// contract is CRD-only, so non-CRD resources are dropped uniformly and the
// chart author is notified, regardless of CRDsPolicy.
func Test_applyCRDs_CreateFiltersNonCRDsAndWarns(t *testing.T) {
	g := NewWithT(t)

	ctx, cfg, kc, api, ctxLog := newTestCfg(t)

	err := applyCRDs(ctx, cfg, v2.Create, testChart(), nil, true, helmkube.StatusWatcherStrategy, nil)
	g.Expect(err).ToNot(HaveOccurred())

	// Only the CRD reaches KubeClient.Create; the VAP is filtered out.
	g.Expect(kc.createCalls).To(HaveLen(1))
	g.Expect(kc.createCalls[0][0].Mapping.GroupVersionKind.Kind).To(Equal("CustomResourceDefinition"))
	g.Expect(kc.createCalls[0][0].Name).To(Equal("foos.example.com"))

	// Same warning as CreateReplace — emitted in one place, for both policies.
	logOut := ctxLog.String()
	g.Expect(logOut).To(ContainSubstring(expectedWarningMessage),
		"Create policy must emit the non-CRD warning too")
	g.Expect(logOut).To(ContainSubstring(expectedNonCRDJSON))

	// With server-side apply the CRD is looked up first, so an existing CRD
	// can be skipped instead of upserted.
	g.Expect(api.GetPaths()).To(HaveLen(1))
	g.Expect(api.GetPaths()[0]).To(ContainSubstring("foos.example.com"))
}

// With the Create policy and server-side apply, an already existing CRD must
// not be re-applied: the existence check turns it into the same skip that
// client-side apply gets from the apiserver's AlreadyExists error.
func Test_applyCRDs_CreateSSASkipsExistingCRDs(t *testing.T) {
	g := NewWithT(t)

	ctx, cfg, kc, api, _ := newTestCfg(t)
	api.AddCRD("foos.example.com")

	err := applyCRDs(ctx, cfg, v2.Create, testChart(), nil, true, helmkube.StatusWatcherStrategy, nil)
	g.Expect(err).ToNot(HaveOccurred())

	// The CRD was looked up via apiextensions...
	g.Expect(api.GetPaths()).To(HaveLen(1))
	g.Expect(api.GetPaths()[0]).To(ContainSubstring("foos.example.com"))

	// ...found to exist, and therefore never applied.
	g.Expect(kc.createCalls).To(BeEmpty())
}

// Client-side apply relies on the apiserver to reject existing CRDs with an
// AlreadyExists error, so the pre-apply existence check must not run.
func Test_applyCRDs_CreateCSADoesNotLookUpCRDs(t *testing.T) {
	g := NewWithT(t)

	ctx, cfg, kc, api, _ := newTestCfg(t)
	api.AddCRD("foos.example.com")

	err := applyCRDs(ctx, cfg, v2.Create, testChart(), nil, false, helmkube.StatusWatcherStrategy, nil)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(api.GetPaths()).To(BeEmpty())
	g.Expect(kc.createCalls).To(HaveLen(1))
}

// With CreateReplace, the structurally CRD-only path must reject non-CRDs
// outright: they're filtered before any apiextensions client.Get(), and a
// loud warning is emitted that names them.
func Test_applyCRDs_CreateReplaceFiltersNonCRDsAndWarns(t *testing.T) {
	g := NewWithT(t)

	ctx, cfg, kc, api, ctxLog := newTestCfg(t)

	err := applyCRDs(ctx, cfg, v2.CreateReplace, testChart(), nil, true, helmkube.StatusWatcherStrategy, nil)
	g.Expect(err).ToNot(HaveOccurred())

	// The warning is logged with the exact message and the VAP ref in the
	// JSON-encoded structured attribute. Asserted on the controller-runtime
	// logger output (ctrl.LoggerFrom(ctx)) -- emitted to pod stdout on every
	// reconcile, not gated on failure like Helm's debug LogBuffer.
	logOut := ctxLog.String()
	g.Expect(logOut).To(ContainSubstring(expectedWarningMessage),
		"expected the warning message to appear verbatim in the ctx logger output")
	g.Expect(logOut).To(ContainSubstring(expectedNonCRDJSON),
		"expected the nonCRDs structured attribute to be rendered as JSON")

	// The non-CRD must never have reached the apiextensions client.Get().
	for _, p := range api.GetPaths() {
		g.Expect(p).ToNot(ContainSubstring("safe-upgrades.example.com"),
			"non-CRD resource name leaked into apiextensions GET: %s", p)
	}

	// Only the CRD reaches the downstream Update call.
	g.Expect(kc.updateCalls).To(HaveLen(1))
	g.Expect(kc.updateCalls[0].Target).To(HaveLen(1))
	gvk := kc.updateCalls[0].Target[0].Mapping.GroupVersionKind
	g.Expect(gvk.Group).To(Equal("apiextensions.k8s.io"))
	g.Expect(gvk.Kind).To(Equal("CustomResourceDefinition"))
	g.Expect(kc.updateCalls[0].Target[0].Name).To(Equal("foos.example.com"))

	// And the CRD was queried via apiextensions, as we'd expect.
	g.Expect(api.GetPaths()).ToNot(BeEmpty(),
		"CreateReplace should have queried the CRD via apiextensions")
	for _, p := range api.GetPaths() {
		g.Expect(p).To(ContainSubstring("foos.example.com"),
			"unexpected GET path %s", p)
	}
}
