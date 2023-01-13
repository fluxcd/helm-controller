package runner

import (
	"bytes"
	"reflect"
	"testing"
)

const aliasesAnchorsMergeKeyResourceMock = `---
apiVersion: v1
kind: Service
metadata:
  name: service-without-annotations
---
x-first-level-annotations:
  annotations: &first_level_annotations
    first_level: annotation
    override_by_second_level: originally_set_by_first_level
    override_by_third_level: originally_set_by_first_level
x-second-level-annotations:
  annotations: &second_level_annotations
    <<: *first_level_annotations
    second_level: annotation
    override_by_second_level: set_by_second_level
    override_by_third_level: set_by_second_level
apiVersion: v1
kind: Service
metadata:
  name: service-with-annotations
annotations:
  <<: *second_level_annotations
  third_level: annotation
  override_by_third_level: set_by_third_level
`

func Test_postRendererYamlFlatten_Run(t *testing.T) {
	tests := []struct {
		name              string
		renderedManifests string
		expectManifests   string
		expectErr         bool
	}{
		{
			name:              "flattened-yaml",
			renderedManifests: aliasesAnchorsMergeKeyResourceMock,
			expectManifests: `apiVersion: v1
kind: Service
metadata:
    name: service-without-annotations
---
annotations:
    first_level: annotation
    override_by_second_level: set_by_second_level
    override_by_third_level: set_by_third_level
    second_level: annotation
    third_level: annotation
apiVersion: v1
kind: Service
metadata:
    name: service-with-annotations
x-first-level-annotations:
    annotations:
        first_level: annotation
        override_by_second_level: originally_set_by_first_level
        override_by_third_level: originally_set_by_first_level
x-second-level-annotations:
    annotations:
        first_level: annotation
        override_by_second_level: set_by_second_level
        override_by_third_level: set_by_second_level
        second_level: annotation
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &postRendererYamlFlatten{
				name:      "name",
				namespace: "namespace",
			}
			gotModifiedManifests, err := k.Run(bytes.NewBufferString(tt.renderedManifests))
			if (err != nil) != tt.expectErr {
				t.Errorf("Run() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !reflect.DeepEqual(gotModifiedManifests, bytes.NewBufferString(tt.expectManifests)) {
				t.Errorf("Run() gotModifiedManifests = \"%v\", want \"%v\"", gotModifiedManifests, tt.expectManifests)
			}
		})
	}
}
