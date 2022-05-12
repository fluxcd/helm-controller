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
)

// ConfigFromSecret returns the KubeConfig data from the provided key in the
// given Secret, or attempts to load the data from the default `value` and
// `value.yaml` keys. If a Secret is provided but no key with data can be
// found, an error is returned. The secret may be nil, in which case no bytes
// nor error are returned. Validation of the data is expected to happen while
// decoding the bytes.
func ConfigFromSecret(secret *corev1.Secret, key string) ([]byte, error) {
	var kubeConfig []byte
	if secret != nil {
		secretName := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
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
	}
	return kubeConfig, nil
}
