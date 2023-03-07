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

package kube

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fluxcd/pkg/runtime/client"
)

const (
	// DefaultKubeConfigSecretKey is the default data key ConfigFromSecret
	// looks at when no data key is provided.
	DefaultKubeConfigSecretKey = "value"
	// DefaultKubeConfigSecretKeyExt is the default data key ConfigFromSecret
	// looks at when no data key is provided, and DefaultKubeConfigSecretKey
	// does not exist.
	DefaultKubeConfigSecretKeyExt = DefaultKubeConfigSecretKey + ".yaml"
)

// ConfigFromSecret returns the KubeConfig data from the provided key in the
// given Secret, or attempts to load the data from the default `value` and
// `value.yaml` keys. If a Secret is provided but no key with data can be
// found, an error is returned.
func ConfigFromSecret(secret *corev1.Secret, key string, opts client.KubeConfigOptions) (*rest.Config, error) {
	if secret == nil {
		return nil, fmt.Errorf("KubeConfig secret is nil")
	}

	var (
		kubeConfig []byte
		secretName = fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
	)
	switch {
	case key != "":
		kubeConfig = secret.Data[key]
		if kubeConfig == nil {
			return nil, fmt.Errorf("KubeConfig secret '%s' does not contain a '%s' key with data", secretName, key)
		}
	case secret.Data[DefaultKubeConfigSecretKey] != nil:
		kubeConfig = secret.Data[DefaultKubeConfigSecretKey]
	case secret.Data[DefaultKubeConfigSecretKeyExt] != nil:
		kubeConfig = secret.Data[DefaultKubeConfigSecretKeyExt]
	default:
		// User did not specify a key, and the 'value' key was not defined.
		return nil, fmt.Errorf("KubeConfig secret '%s' does not contain a '%s' or '%s' key with data", secretName, DefaultKubeConfigSecretKey, DefaultKubeConfigSecretKeyExt)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load KubeConfig from secret '%s': %w", secretName, err)
	}
	cfg = client.KubeConfig(cfg, opts)
	return cfg, nil
}
