/*
Copyright 2026 The Flux authors

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

package postrender

import (
	"bytes"
	"fmt"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/kio"
)

// inflateYAMLAliases expands YAML anchors/aliases in a multi-document stream.
//
// Helm v4's post-render merge (annotateAndMerge) unwraps kind:List into
// individual documents while preserving YAML anchors. That turns in-List
// aliases into cross-document references, which standard YAML parsers reject
// ("unknown anchor referenced"). Re-wrapping into a List restores a single
// document scope so aliases resolve, then DeAnchor inlines them permanently.
//
// If the input already parses cleanly, it is returned unchanged to avoid
// reformatting manifests.
func inflateYAMLAliases(in []byte) ([]byte, error) {
	if len(bytes.TrimSpace(in)) == 0 {
		return in, nil
	}

	if _, err := kio.ParseAll(string(in)); err == nil {
		return in, nil
	} else if !isUnknownAnchorErr(err) {
		return nil, err
	}

	wrapped := wrapDocsAsList(string(in))
	nodes, err := kio.ParseAll(wrapped)
	if err != nil {
		return nil, fmt.Errorf("inflate YAML aliases: %w", err)
	}
	for _, n := range nodes {
		if err := n.DeAnchor(); err != nil {
			return nil, fmt.Errorf("inflate YAML aliases: %w", err)
		}
	}
	out, err := kio.StringAll(nodes)
	if err != nil {
		return nil, fmt.Errorf("inflate YAML aliases: %w", err)
	}
	return []byte(out), nil
}

func isUnknownAnchorErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown anchor")
}

// wrapDocsAsList nests each YAML document in the stream as an item of a
// synthetic List so anchors/aliases share one document scope.
func wrapDocsAsList(in string) string {
	docs := splitYAMLDocuments(in)
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: List\nitems:\n")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		lines := strings.Split(doc, "\n")
		for i, line := range lines {
			if i == 0 {
				b.WriteString("- ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func splitYAMLDocuments(in string) []string {
	raw := strings.Split(in, "\n")
	var (
		docs  []string
		cur   strings.Builder
		first = true
	)
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			docs = append(docs, s)
		}
		cur.Reset()
	}
	for _, line := range raw {
		if strings.TrimSpace(line) == "---" {
			flush()
			first = false
			continue
		}
		if !first || cur.Len() > 0 {
			// keep going
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
		}
		cur.WriteString(line)
		first = false
	}
	flush()
	return docs
}
