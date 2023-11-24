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

package errors

import (
	"errors"
	"testing"
)

func TestIsOneOf(t *testing.T) {
	err1 := errors.New("error1")
	err2 := errors.New("error2")

	if !IsOneOf(err1, err1, err2) {
		t.Errorf("Expected IsOneOf to return true when the error is in the list, but got false")
	}

	err3 := errors.New("error3")
	if IsOneOf(err3, err1, err2) {
		t.Errorf("Expected IsOneOf to return false when the error is not in the list, but got true")
	}

	if IsOneOf(err1) {
		t.Errorf("Expected IsOneOf to return false with an empty list of errors, but got true")
	}
}
