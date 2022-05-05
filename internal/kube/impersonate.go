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

	"k8s.io/client-go/rest"
)

// DefaultServiceAccountName can be set at runtime to enable a fallback account
// name when no service account name is provided to SetImpersonationConfig.
var DefaultServiceAccountName string

// userNameFormat is the format of a system service account user name string.
// It formats into `system:serviceaccount:namespace:name`.
const userNameFormat = "system:serviceaccount:%s:%s"

// SetImpersonationConfig configures the provided service account name if
// given, or the DefaultServiceAccountName as a fallback if set. It returns
// the configured impersonation username, or an empty string.
func SetImpersonationConfig(cfg *rest.Config, namespace, serviceAccount string) string {
	name := DefaultServiceAccountName
	if serviceAccount != "" {
		name = serviceAccount
	}
	if name != "" && namespace != "" {
		username := fmt.Sprintf(userNameFormat, namespace, name)
		cfg.Impersonate = rest.ImpersonationConfig{UserName: username}
		return username
	}
	return ""
}
