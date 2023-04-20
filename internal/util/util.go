/*
Copyright 2020 The Flux authors

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

package util

import (
	"crypto/sha1"
	"fmt"
	"sort"

	goyaml "gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/release"
	"sigs.k8s.io/yaml"
)

// ValuesChecksum calculates and returns the SHA1 checksum for the
// given chartutil.Values.
func ValuesChecksum(values map[string]interface{}) string {
	newValues := copyValues(values)
	var (
		s   []byte
		err error
	)

	if len(newValues) != 0 {
		// cleanUpInterfaceMap
		for i, value := range newValues {
			newValues[i] = cleanUpMapValue(value)
		}
		msValues := yaml.JSONObjectToYAMLObject(newValues)
		// Sort
		SortMapSlice(msValues)
		// Marshal
		s, err = goyaml.Marshal(msValues)
		if err != nil {
			panic(err)
		}
	}
	// Gethash
	return fmt.Sprintf("%x", sha1.Sum(s))
}

func SortMapSlice(ms goyaml.MapSlice) {
	sort.Slice(ms, func(i, j int) bool {
		return fmt.Sprint(ms[i].Key) < fmt.Sprint(ms[j].Key)
	})
	for _, item := range ms {
		if nestedMS, ok := item.Value.(goyaml.MapSlice); ok {
			SortMapSlice(nestedMS)
		} else if _, ok := item.Value.([]interface{}); ok {
			for _, vItem := range item.Value.([]interface{}) {
				if itemMS, ok := vItem.(goyaml.MapSlice); ok {
					SortMapSlice(itemMS)
				}
			}
		}
	}
}

func cleanUpMapValue(v interface{}) interface{} {
	switch v := v.(type) {
	case []interface{}:
		return cleanUpInterfaceArray(v)
	case map[interface{}]interface{}:
		return cleanUpInterfaceMap(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func cleanUpInterfaceMap(in map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range in {
		result[fmt.Sprintf("%v", k)] = cleanUpMapValue(v)
	}
	return result
}

func cleanUpInterfaceArray(in []interface{}) []interface{} {
	result := make([]interface{}, len(in))
	for i, v := range in {
		result[i] = cleanUpMapValue(v)
	}
	return result
}

func copyValues(in map[string]interface{}) map[string]interface{} {
	// Marshal
	coppiedValues, err := goyaml.Marshal(in)
	if err != nil {
		panic(err)
	}
	// Unmarshal
	newValues := make(map[string]interface{})
	goyaml.Unmarshal(coppiedValues, newValues)
	return newValues
}

// ReleaseRevision returns the revision of the given release.Release.
func ReleaseRevision(rel *release.Release) int {
	if rel == nil {
		return 0
	}
	return rel.Version
}
