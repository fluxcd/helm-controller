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

package testutil

import (
	"context"
	"log/slog"
)

// MockSLogHandler lets callers know if Handle was called.
type MockSLogHandler struct {
	Called bool
}

// Enabled implements slog.Handler.
func (m *MockSLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// Handle implements slog.Handler.
func (m *MockSLogHandler) Handle(context.Context, slog.Record) error {
	m.Called = true
	return nil
}

// WithAttrs implements slog.Handler.
func (m *MockSLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return m
}

// WithGroup implements slog.Handler.
func (m *MockSLogHandler) WithGroup(name string) slog.Handler {
	return m
}
