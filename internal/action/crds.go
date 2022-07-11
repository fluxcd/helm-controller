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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

const (
	// DefaultCRDPolicy is the default CRD policy.
	DefaultCRDPolicy = helmv2.Create
)

// crdPolicy returns the CRD policy for the given CRD.
func crdPolicyOrDefault(policy helmv2.CRDsPolicy) (helmv2.CRDsPolicy, error) {
	switch policy {
	case "":
		return DefaultCRDPolicy, nil
	case helmv2.Skip:
		break
	case helmv2.Create:
		break
	case helmv2.CreateReplace:
		break
	default:
		return policy, fmt.Errorf("invalid CRD upgrade policy '%s', valid values are '%s', '%s' or '%s'",
			policy, helmv2.Skip, helmv2.Create, helmv2.CreateReplace,
		)
	}
	return policy, nil
}

type rootScoped struct{}

func (*rootScoped) Name() apimeta.RESTScopeName {
	return apimeta.RESTScopeNameRoot
}

func applyCRDs(cfg *helmaction.Configuration, policy helmv2.CRDsPolicy, chrt *helmchart.Chart) error {
	cfg.Log("apply CRDs with policy %s", policy)

	// Collect all CRDs from all files in `crds` directory.
	allCRDs := make(helmkube.ResourceList, 0)
	for _, obj := range chrt.CRDObjects() {
		// Read in the resources
		res, err := cfg.KubeClient.Build(bytes.NewBuffer(obj.File.Data), false)
		if err != nil {
			cfg.Log("failed to parse CRDs from %s: %s", obj.Name, err)
			return fmt.Errorf("failed to parse CRDs from %s: %w", obj.Name, err)
		}
		allCRDs = append(allCRDs, res...)
	}
	var totalItems []*resource.Info
	switch policy {
	case helmv2.Skip:
		break
	case helmv2.Create:
		for i := range allCRDs {
			if rr, err := cfg.KubeClient.Create(allCRDs[i : i+1]); err != nil {
				crdName := allCRDs[i].Name
				// If the error is CRD already exists, continue.
				if apierrors.IsAlreadyExists(err) {
					cfg.Log("CRD %s is already present. Skipping.", crdName)
					if rr != nil && rr.Created != nil {
						totalItems = append(totalItems, rr.Created...)
					}
					continue
				}
				cfg.Log("failed to create CRD %s: %s", crdName, err)
				return fmt.Errorf("failed to create CRD %s: %w", crdName, err)
			} else {
				if rr != nil && rr.Created != nil {
					totalItems = append(totalItems, rr.Created...)
				}
			}
		}
	case helmv2.CreateReplace:
		config, err := cfg.RESTClientGetter.ToRESTConfig()
		if err != nil {
			cfg.Log("Error while creating Kubernetes client config: %s", err)
			return err
		}
		clientset, err := apiextension.NewForConfig(config)
		if err != nil {
			cfg.Log("Error while creating Kubernetes clientset for apiextension: %s", err)
			return err
		}
		client := clientset.ApiextensionsV1().CustomResourceDefinitions()
		original := make(helmkube.ResourceList, 0)
		// Note, we build the originals from the current set of CRDs
		// and therefore this upgrade will never delete CRDs that existed in the former release
		// but no longer exist in the current release.
		for _, r := range allCRDs {
			if o, err := client.Get(context.TODO(), r.Name, metav1.GetOptions{}); err == nil && o != nil {
				o.GetResourceVersion()
				original = append(original, &resource.Info{
					Client: clientset.ApiextensionsV1().RESTClient(),
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
				cfg.Log("failed to get CRD %s: %s", r.Name, err)
				return err
			}
		}
		// Send them to Kube
		if rr, err := cfg.KubeClient.Update(original, allCRDs, true); err != nil {
			cfg.Log("failed to apply CRD %s", err)
			return fmt.Errorf("failed to apply CRD: %w", err)
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
	}
	if len(totalItems) > 0 {
		// Invalidate the local cache, since it will not have the new CRDs
		// present.
		discoveryClient, err := cfg.RESTClientGetter.ToDiscoveryClient()
		if err != nil {
			cfg.Log("Error in cfg.RESTClientGetter.ToDiscoveryClient(): %s", err)
			return err
		}
		cfg.Log("Clearing discovery cache")
		discoveryClient.Invalidate()
		// Give time for the CRD to be recognized.
		if err := cfg.KubeClient.Wait(totalItems, 60*time.Second); err != nil {
			cfg.Log("Error waiting for items: %s", err)
			return err
		}
		// Make sure to force a rebuild of the cache.
		discoveryClient.ServerGroups()
	}
	return nil
}
