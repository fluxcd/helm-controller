/*
Copyright 2023 The Flux authors

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

package acl

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/runtime/acl"
)

var (
	// AllowCrossNamespaceRef is a global flag that can be used to allow
	// cross-namespace references.
	AllowCrossNamespaceRef = false
)

// AllowsAccessTo returns an error if the object does not allow access to the
// given reference.
func AllowsAccessTo(obj client.Object, kind string, ref types.NamespacedName) error {
	if !AllowCrossNamespaceRef && obj.GetNamespace() != ref.Namespace {
		return acl.AccessDeniedError(fmt.Sprintf("cross-namespace references are not allowed: cannot access %s %s",
			kind, ref.String(),
		))
	}
	return nil
}
