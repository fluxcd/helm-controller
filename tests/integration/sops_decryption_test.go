package integration

import (
	"context"
	"testing"
)

// NOTE: Integration test skeleton. To run:
// - Build controller image containing `sops` and push to registry.
// - Deploy controller image to the Kind/cluster used for tests.
// - Create Kubernetes Secret(s) / ServiceAccount per your KMS provider.
// - Create a HelmRelease that references a sops-encrypted values file and sets `spec.decryption`.
//
// This test verifies that a HelmRelease with sops-encrypted values is successfully applied.
func TestSOPSDecryptionIntegration(t *testing.T) {
	t.Skip("integration test skeleton: implement cluster setup and controller image with sops")
	ctx := context.Background()

	_ = ctx
	// Steps:
	// 1. Create secret(s) needed for sops/KMS access.
	// 2. Apply HelmRelease CR with `spec.decryption: { provider: sops, secretRef: { name: ... } }`.
	// 3. Wait for HelmRelease Ready condition or release presence.
	// 4. Assert that the applied resources contain expected plaintext values.
}
