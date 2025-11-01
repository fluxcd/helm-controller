/*
Copyright 2025 The Flux authors

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

package reconcile

import (
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	helper "github.com/fluxcd/pkg/runtime/controller"
)

// getFailureReasonAndMessage determines the appropriate failure reason and
// message based on the provided error and context. If the error is due to a
// new reconciliation being triggered, it adjusts the reason and message
// accordingly.
func getFailureReasonAndMessage(ctx context.Context, err error, reason string) (string, string) {
	errMsg := err.Error()
	if is, qes := helper.IsObjectEnqueued(ctx); errors.Is(err, context.Canceled) && is {
		reason = meta.HealthCheckCanceledReason
		errMsg = fmt.Sprintf("New reconciliation triggered by %s", qes.Error())
	}
	return reason, errMsg
}
