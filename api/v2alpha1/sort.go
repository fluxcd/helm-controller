/*
Copyright 2020 The Flux CD contributors.

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

package v2alpha1

import (
	"fmt"
)

// CircularDependencyError contains the circular dependency chains
// that were detected while sorting 'HelmReleaseSpec.DependsOn'.
// +kubebuilder:object:generate=false
type CircularDependencyError [][]string

func (e CircularDependencyError) Error() string {
	return fmt.Sprintf("circular dependencies: %v", [][]string(e))
}

// DependencySort sorts the slice of HelmReleases based on their listed
// dependencies using Tarjan's strongly connected components algorithm.
func DependencySort(ks []HelmRelease) ([]HelmRelease, error) {
	n := make(graph)
	lookup := map[string]*HelmRelease{}
	for i := 0; i < len(ks); i++ {
		n[ks[i].Name] = after(ks[i].Spec.DependsOn)
		lookup[ks[i].Name] = &ks[i]
	}
	sccs := tarjanSCC(n)
	var sorted []HelmRelease
	var unsortable CircularDependencyError
	for i := 0; i < len(sccs); i++ {
		s := sccs[i]
		if len(s) != 1 {
			unsortable = append(unsortable, s)
			continue
		}
		if k, ok := lookup[s[0]]; ok {
			sorted = append(sorted, *k.DeepCopy())
		}
	}
	if unsortable != nil {
		for i, j := 0, len(unsortable)-1; i < j; i, j = i+1, j-1 {
			unsortable[i], unsortable[j] = unsortable[j], unsortable[i]
		}
		return nil, unsortable
	}
	return sorted, nil
}

// https://en.wikipedia.org/wiki/Tarjan%27s_strongly_connected_components_algorithm#The_algorithm_in_pseudocode

type graph map[string]edges

type edges map[string]struct{}

func after(i []string) edges {
	if len(i) == 0 {
		return nil
	}
	s := make(edges)
	for _, v := range i {
		s[v] = struct{}{}
	}
	return s
}

func tarjanSCC(g graph) [][]string {
	t := tarjan{
		g: g,

		indexTable: make(map[string]int, len(g)),
		lowLink:    make(map[string]int, len(g)),
		onStack:    make(map[string]bool, len(g)),
	}
	for v := range t.g {
		if t.indexTable[v] == 0 {
			t.strongconnect(v)
		}
	}
	return t.sccs
}

type tarjan struct {
	g graph

	index      int
	indexTable map[string]int
	lowLink    map[string]int
	onStack    map[string]bool

	stack []string

	sccs [][]string
}

func (t *tarjan) strongconnect(v string) {
	// Set the depth index for v to the smallest unused index.
	t.index++
	t.indexTable[v] = t.index
	t.lowLink[v] = t.index
	t.stack = append(t.stack, v)
	t.onStack[v] = true

	// Consider successors of v.
	for w := range t.g[v] {
		if t.indexTable[w] == 0 {
			// Successor w has not yet been visited; recur on it.
			t.strongconnect(w)
			t.lowLink[v] = min(t.lowLink[v], t.lowLink[w])
		} else if t.onStack[w] {
			// Successor w is in stack s and hence in the current SCC.
			t.lowLink[v] = min(t.lowLink[v], t.indexTable[w])
		}
	}

	// If v is a root graph, pop the stack and generate an SCC.
	if t.lowLink[v] == t.indexTable[v] {
		// Start a new strongly connected component.
		var (
			scc []string
			w   string
		)
		for {
			w, t.stack = t.stack[len(t.stack)-1], t.stack[:len(t.stack)-1]
			t.onStack[w] = false
			// Add w to current strongly connected component.
			scc = append(scc, w)
			if w == v {
				break
			}
		}
		// Output the current strongly connected component.
		t.sccs = append(t.sccs, scc)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
