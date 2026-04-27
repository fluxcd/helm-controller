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

package controller

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"gopkg.in/yaml.v3"
	ctrl "sigs.k8s.io/controller-runtime"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// runSops performs SOPS decryption of the provided YAML bytes.
// Overridable for unit testing.
var runSops = func(ctx context.Context, in []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sops", "--decrypt", "--output-type", "yaml", "-")
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sops decrypt failed: %w: %s", err, string(out))
	}
	return out, nil
}

// decryptValues decrypts composed Helm values when `obj.Spec.Decryption` is set.
// It marshals `values` to YAML, pipes through `sops --decrypt` and unmarshals the result.
// For production, prefer reusing fluxcd/pkg/kustomize/kustomize-controller decryption logic.
func (r *HelmReleaseReconciler) decryptValues(ctx context.Context, obj *v2.HelmRelease, values map[string]any) (map[string]any, error) {
	log := ctrl.LoggerFrom(ctx)

	if values == nil {
		return nil, fmt.Errorf("values are nil")
	}

	in, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal values: %w", err)
	}

	out, err := runSops(ctx, in)
	if err != nil {
		// avoid exposing secret contents
		log.Info("sops decryption failed")
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	var dec map[string]any
	if err := yaml.Unmarshal(out, &dec); err != nil {
		return nil, fmt.Errorf("unmarshal decrypted values: %w", err)
	}

	return dec, nil
}

// DecryptValues is an exported wrapper for testing and external usage that
// delegates to the reconciler's decryptValues method.
func DecryptValues(ctx context.Context, r *HelmReleaseReconciler, obj *v2.HelmRelease, values map[string]any) (map[string]any, error) {
	return r.decryptValues(ctx, obj, values)
}

// SetSopsRunner allows tests to override the sops runner implementation.
func SetSopsRunner(runner func(ctx context.Context, in []byte) ([]byte, error)) {
	runSops = runner
}
