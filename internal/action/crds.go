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

package action

import (
	"bytes"
	"context"
	"fmt"
	"time"

	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmkube "helm.sh/helm/v3/pkg/kube"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

const (
	// DefaultCRDPolicy is the default CRD policy.
	DefaultCRDPolicy = v2.Create
)

var accessor = apimeta.NewAccessor()

// crdPolicy returns the CRD policy for the given CRD.
func crdPolicyOrDefault(policy v2.CRDsPolicy) (v2.CRDsPolicy, error) {
	switch policy {
	case "":
		policy = DefaultCRDPolicy
	case v2.Skip, v2.Create, v2.CreateReplace:
		break
	default:
		return policy, fmt.Errorf("invalid CRD upgrade policy '%s', valid values are '%s', '%s' or '%s'",
			policy, v2.Skip, v2.Create, v2.CreateReplace,
		)
	}
	return policy, nil
}

type rootScoped struct{}

func (*rootScoped) Name() apimeta.RESTScopeName {
	return apimeta.RESTScopeNameRoot
}

func applyCRDs(cfg *helmaction.Configuration, policy v2.CRDsPolicy, chrt *helmchart.Chart, visitorFunc ...resource.VisitorFunc) error {
	if len(chrt.CRDObjects()) == 0 {
		return nil
	}

	if policy == v2.Skip {
		cfg.Log("skipping CustomResourceDefinition apply: policy is set to %s", policy)
		return nil
	}

	// Collect all CRDs from all files in `crds` directory.
	allCRDs := make(helmkube.ResourceList, 0)
	for _, obj := range chrt.CRDObjects() {
		// Read in the resources
		res, err := cfg.KubeClient.Build(bytes.NewBuffer(obj.File.Data), false)
		if err != nil {
			err = fmt.Errorf("failed to parse CustomResourceDefinitions from %s: %w", obj.Name, err)
			cfg.Log(err.Error())
			return err
		}
		allCRDs = append(allCRDs, res...)
	}

	// Visit CRDs with any provided visitor functions.
	for _, visitor := range visitorFunc {
		if err := allCRDs.Visit(visitor); err != nil {
			return err
		}
	}

	cfg.Log("applying CustomResourceDefinition(s) with policy %s", policy)
	var totalItems []*resource.Info
	switch policy {
	case v2.Create:
		for i := range allCRDs {
			if rr, err := cfg.KubeClient.Create(allCRDs[i : i+1]); err != nil {
				crdName := allCRDs[i].Name
				// If the CustomResourceDefinition already exists, we skip it.
				if apierrors.IsAlreadyExists(err) {
					cfg.Log("CustomResourceDefinition %s is already present. Skipping.", crdName)
					if rr != nil && rr.Created != nil {
						totalItems = append(totalItems, rr.Created...)
					}
					continue
				}
				err = fmt.Errorf("failed to create CustomResourceDefinition %s: %w", crdName, err)
				cfg.Log(err.Error())
				return err
			} else {
				if rr != nil && rr.Created != nil {
					totalItems = append(totalItems, rr.Created...)
				}
			}
		}
	case v2.CreateReplace:
		config, err := cfg.RESTClientGetter.ToRESTConfig()
		if err != nil {
			err = fmt.Errorf("could not create Kubernetes client REST config: %w", err)
			cfg.Log(err.Error())
			return err
		}
		clientSet, err := apiextension.NewForConfig(config)
		if err != nil {
			err = fmt.Errorf("could not create Kubernetes client set for API extensions: %w", err)
			cfg.Log(err.Error())
			return err
		}
		client := clientSet.ApiextensionsV1().CustomResourceDefinitions()

		// Note, we build the originals from the current set of Custom Resource
		// Definitions, and therefore this upgrade will never delete CRDs that
		// existed in the former release but no longer exist in the current
		// release.
		original := make(helmkube.ResourceList, 0)
		for _, r := range allCRDs {
			if o, err := client.Get(context.TODO(), r.Name, metav1.GetOptions{}); err == nil && o != nil {
				o.GetResourceVersion()
				original = append(original, &resource.Info{
					Client: clientSet.ApiextensionsV1().RESTClient(),
					Mapping: &apimeta.RESTMapping{
						Resource: schema.GroupVersionResource{
							Group:    "apiextensions.k8s.io",
							Version:  r.Mapping.GroupVersionKind.Version,
							Resource: "customresourcedefinition",
						},
						GroupVersionKind: schema.GroupVersionKind{
							Kind:    "CustomResourceDefinition",
							Group:   "apiextensions.k8s.io",
							Version: r.Mapping.GroupVersionKind.Version,
						},
						Scope: &rootScoped{},
					},
					Namespace:       o.ObjectMeta.Namespace,
					Name:            o.ObjectMeta.Name,
					Object:          o,
					ResourceVersion: o.ObjectMeta.ResourceVersion,
				})
			} else if !apierrors.IsNotFound(err) {
				err = fmt.Errorf("failed to get CustomResourceDefinition %s: %w", r.Name, err)
				cfg.Log(err.Error())
				return err
			}
		}

		// Send them to Kubernetes...
		if rr, err := cfg.KubeClient.Update(original, allCRDs, true); err != nil {
			err = fmt.Errorf("failed to update CustomResourceDefinition(s): %w", err)
			return err
		} else {
			if rr != nil {
				if rr.Created != nil {
					totalItems = append(totalItems, rr.Created...)
				}
				if rr.Updated != nil {
					totalItems = append(totalItems, rr.Updated...)
				}
				if rr.Deleted != nil {
					totalItems = append(totalItems, rr.Deleted...)
				}
			}
		}
	default:
		err := fmt.Errorf("unexpected policy %s", policy)
		cfg.Log(err.Error())
		return err
	}

	if len(totalItems) > 0 {
		// Give time for the CRD to be recognized.
		if err := cfg.KubeClient.Wait(totalItems, 60*time.Second); err != nil {
			err = fmt.Errorf("failed to wait for CustomResourceDefinition(s): %w", err)
			cfg.Log(err.Error())
			return err
		}
		cfg.Log("successfully applied %d CustomResourceDefinition(s)", len(totalItems))

		// Clear the RESTMapper cache, since it will not have the new CRDs.
		// Helm does further invalidation of the client at a later stage
		// when it gathers the server capabilities.
		if m, err := cfg.RESTClientGetter.ToRESTMapper(); err == nil {
			if rm, ok := m.(apimeta.ResettableRESTMapper); ok {
				cfg.Log("clearing REST mapper cache")
				rm.Reset()
			}
		}
	}

	return nil
}

func setOriginVisitor(group, namespace, name string) resource.VisitorFunc {
	return func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		if err = mergeLabels(info.Object, originLabels(group, namespace, name)); err != nil {
			return fmt.Errorf(
				"%s origin labels could not be updated: %s",
				resourceString(info), err,
			)
		}
		return nil
	}
}

func originLabels(group, namespace, name string) map[string]string {
	return map[string]string{
		fmt.Sprintf("%s/name", group):      name,
		fmt.Sprintf("%s/namespace", group): namespace,
	}
}

func mergeLabels(obj apiruntime.Object, labels map[string]string) error {
	current, err := accessor.Labels(obj)
	if err != nil {
		return err
	}
	return accessor.SetLabels(obj, mergeStrStrMaps(current, labels))
}

func resourceString(info *resource.Info) string {
	_, k := info.Mapping.GroupVersionKind.ToAPIVersionAndKind()
	return fmt.Sprintf(
		"%s %q in namespace %q",
		k, info.Name, info.Namespace,
	)
}

func mergeStrStrMaps(current, desired map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range current {
		result[k] = v
	}
	for k, desiredVal := range desired {
		result[k] = desiredVal
	}
	return result
}
