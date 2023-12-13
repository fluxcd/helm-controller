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

package loader

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NewRetryableHTTPClient returns a new retrying HTTP client for loading
// artifacts. The client will retry up to the given number of times before
// giving up. The context is used to log errors.
func NewRetryableHTTPClient(ctx context.Context, retries int) *retryablehttp.Client {
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = retries
	httpClient.Logger = newLoggerForContext(ctx)
	return httpClient
}

func newLoggerForContext(ctx context.Context) retryablehttp.LeveledLogger {
	return &errorLogger{log: ctrl.LoggerFrom(ctx)}
}

// errorLogger is a wrapper around logr.Logger that implements the
// retryablehttp.LeveledLogger interface while only logging errors.
type errorLogger struct {
	log logr.Logger
}

func (l *errorLogger) Error(msg string, keysAndValues ...interface{}) {
	l.log.Info(msg, keysAndValues...)
}

func (l *errorLogger) Info(msg string, keysAndValues ...interface{}) {
	// Do nothing.
}

func (l *errorLogger) Debug(msg string, keysAndValues ...interface{}) {
	// Do nothing.
}

func (l *errorLogger) Warn(msg string, keysAndValues ...interface{}) {
	// Do nothing.
}
