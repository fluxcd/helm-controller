package controller_test

import (
	"context"
	"reflect"
	"testing"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	ctrlpkg "github.com/fluxcd/helm-controller/internal/controller"
)

func TestDecryptValues_Success(t *testing.T) {
	// Override runSops to return a simple decrypted YAML
	ctrlpkg.SetSopsRunner(func(ctx context.Context, in []byte) ([]byte, error) {
		return []byte("alpha: 1\nbeta:\n  nested: xyz\n"), nil
	})

	r := &ctrlpkg.HelmReleaseReconciler{}
	obj := &v2.HelmRelease{
		Spec: v2.HelmReleaseSpec{
			Decryption: &v2.Decryption{}, // presence is enough for this test
		},
	}

	input := map[string]any{
		"alpha": 123,
	}

	got, err := ctrlpkg.DecryptValues(context.Background(), r, obj, input)
	if err != nil {
		t.Fatalf("DecryptValues returned error: %v", err)
	}

	want := map[string]any{
		"alpha": int(1),
		"beta": map[string]any{
			"nested": "xyz",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected decrypted values.\nwant: %#v\ngot:  %#v", want, got)
	}
}
