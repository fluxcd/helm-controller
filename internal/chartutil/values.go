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

package chartutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/runtime/transform"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

// ErrValuesRefReason is the descriptive reason for an ErrValuesReference.
type ErrValuesRefReason error

var (
	// ErrResourceNotFound signals the referenced values resource could not be
	// found.
	ErrResourceNotFound = errors.New("resource not found")
	// ErrKeyNotFound signals the key could not be found in the referenced
	// values resource.
	ErrKeyNotFound = errors.New("key not found")
	// ErrUnsupportedRefKind signals the values reference kind is not
	// supported.
	ErrUnsupportedRefKind = errors.New("unsupported values reference kind")
	// ErrValuesDataRead signals the referenced resource's values data could
	// not be read.
	ErrValuesDataRead = errors.New("failed to read values data")
	// ErrValueMerge signals a single value could not be merged into the
	// values.
	ErrValueMerge = errors.New("failed to merge value")
	// ErrUnknown signals the reason an error occurred is unknown.
	ErrUnknown = errors.New("unknown error")
)

// ErrValuesReference is returned by ChartValuesFromReferences
type ErrValuesReference struct {
	// Reason for the values reference error. Nil equals ErrUnknown.
	// Can be used with Is to reason about a returned error:
	//  err := &ErrValuesReference{Reason: ErrResourceNotFound, ...}
	//  errors.Is(err, ErrResourceNotFound)
	Reason ErrValuesRefReason
	// Kind of the values reference the error is being reported for.
	Kind string
	// Name of the values reference the error is being reported for.
	Name types.NamespacedName
	// Key of the values reference the error is being reported for.
	Key string
	// Optional indicates if the error is being reported for an optional values
	// reference.
	Optional bool
	// Err contains the further error chain leading to this error, it can be
	// nil.
	Err error
}

// Error returns an error string constructed out of the state of
// ErrValuesReference.
func (e *ErrValuesReference) Error() string {
	b := strings.Builder{}
	b.WriteString("could not resolve")
	if e.Optional {
		b.WriteString(" optional")
	}
	if kind := e.Kind; kind != "" {
		b.WriteString(" " + kind)
	}
	b.WriteString(" chart values reference")
	if name := e.Name.String(); name != "" {
		b.WriteString(fmt.Sprintf(" '%s'", name))
	}
	if key := e.Key; key != "" {
		b.WriteString(fmt.Sprintf(" with key '%s'", key))
	}
	reason := e.Reason.Error()
	if reason == "" && e.Err == nil {
		reason = ErrUnknown.Error()
	}
	if e.Err != nil {
		reason = e.Err.Error()
	}
	b.WriteString(": " + reason)
	return b.String()
}

// Is returns if target == Reason, or target == Err.
// Can be used to Reason about a returned error:
//
//	err := &ErrValuesReference{Reason: ErrResourceNotFound, ...}
//	errors.Is(err, ErrResourceNotFound)
func (e *ErrValuesReference) Is(target error) bool {
	reason := e.Reason
	if reason == nil {
		reason = ErrUnknown
	}
	if reason == target {
		return true
	}
	return errors.Is(e.Err, target)
}

// Unwrap returns the wrapped Err.
func (e *ErrValuesReference) Unwrap() error {
	return e.Err
}

// NewErrValuesReference returns a new ErrValuesReference constructed from the
// provided values.
func NewErrValuesReference(name types.NamespacedName, ref v2.ValuesReference, reason ErrValuesRefReason, err error) *ErrValuesReference {
	return &ErrValuesReference{
		Reason:   reason,
		Kind:     ref.Kind,
		Name:     name,
		Key:      ref.GetValuesKey(),
		Optional: ref.Optional,
		Err:      err,
	}
}

const (
	kindConfigMap = "ConfigMap"
	kindSecret    = "Secret"
)

// ChartValuesFromReferences attempts to construct new chart values by resolving
// the provided references using the client, merging them in the order given.
// If provided, the values map is merged in last. Overwriting values from
// references. It returns the merged values, or an ErrValuesReference error.
func ChartValuesFromReferences(ctx context.Context, client kubeclient.Client, namespace string,
	values map[string]interface{}, refs ...v2.ValuesReference) (chartutil.Values, error) {

	log := ctrl.LoggerFrom(ctx)

	result := chartutil.Values{}
	resources := make(map[string]kubeclient.Object)

	for _, ref := range refs {
		namespacedName := types.NamespacedName{Namespace: namespace, Name: ref.Name}
		var valuesData []byte

		switch ref.Kind {
		case kindConfigMap, kindSecret:
			index := ref.Kind + namespacedName.String()

			resource, ok := resources[index]
			if !ok {
				// The resource may not exist, but we want to act on a single version
				// of the resource in case the values reference is marked as optional.
				resources[index] = nil

				switch ref.Kind {
				case kindSecret:
					resource = &corev1.Secret{}
				case kindConfigMap:
					resource = &corev1.ConfigMap{}
				}

				if resource != nil {
					if err := client.Get(ctx, namespacedName, resource); err != nil {
						if apierrors.IsNotFound(err) {
							err := NewErrValuesReference(namespacedName, ref, ErrResourceNotFound, err)
							if err.Optional {
								log.Info(err.Error())
								continue
							}
							return nil, err
						}
						return nil, err
					}
					resources[index] = resource
				}
			}

			if resource == nil {
				if ref.Optional {
					continue
				}
				return nil, NewErrValuesReference(namespacedName, ref, ErrResourceNotFound, nil)
			}

			switch typedRes := resource.(type) {
			case *corev1.Secret:
				data, ok := typedRes.Data[ref.GetValuesKey()]
				if !ok {
					err := NewErrValuesReference(namespacedName, ref, ErrKeyNotFound, nil)
					if ref.Optional {
						log.Info(err.Error())
						continue
					}
					return nil, NewErrValuesReference(namespacedName, ref, ErrKeyNotFound, nil)
				}
				valuesData = data
			case *corev1.ConfigMap:
				data, ok := typedRes.Data[ref.GetValuesKey()]
				if !ok {
					err := NewErrValuesReference(namespacedName, ref, ErrKeyNotFound, nil)
					if ref.Optional {
						log.Info(err.Error())
						continue
					}
					return nil, err
				}
				valuesData = []byte(data)
			default:
				return nil, NewErrValuesReference(namespacedName, ref, ErrUnsupportedRefKind, nil)
			}
		default:
			return nil, NewErrValuesReference(namespacedName, ref, ErrUnsupportedRefKind, nil)
		}

		if ref.TargetPath != "" {
			// TODO(hidde): this is a bit of hack, as it mimics the way the option string is passed
			// 	to Helm from a CLI perspective. Given the parser is however not publicly accessible
			// 	while it contains all logic around parsing the target path, it is a fair trade-off.
			if err := ReplacePathValue(result, ref.TargetPath, string(valuesData)); err != nil {
				return nil, NewErrValuesReference(namespacedName, ref, ErrValueMerge, err)
			}
			continue
		}

		values, err := chartutil.ReadValues(valuesData)
		if err != nil {
			return nil, NewErrValuesReference(namespacedName, ref, ErrValuesDataRead, err)
		}
		result = transform.MergeMaps(result, values)
	}
	return transform.MergeMaps(result, values), nil
}

// ReplacePathValue replaces the value at the dot notation path with the given
// value using Helm's string value parser using strvals.ParseInto. Single or
// double-quoted values are merged using strvals.ParseIntoString.
func ReplacePathValue(values chartutil.Values, path string, value string) error {
	const (
		singleQuote = "'"
		doubleQuote = `"`
	)
	isSingleQuoted := strings.HasPrefix(value, singleQuote) && strings.HasSuffix(value, singleQuote)
	isDoubleQuoted := strings.HasPrefix(value, doubleQuote) && strings.HasSuffix(value, doubleQuote)
	if isSingleQuoted || isDoubleQuoted {
		value = strings.Trim(value, singleQuote+doubleQuote)
		value = path + "=" + value
		return strvals.ParseIntoString(value, values)
	}
	value = path + "=" + value
	return strvals.ParseInto(value, values)
}
