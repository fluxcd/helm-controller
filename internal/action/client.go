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

package action

import (
	"context"

	helmkube "helm.sh/helm/v4/pkg/kube"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/fluxcd/pkg/ssa"
)

// Client wraps a Helm kube Client to replace the kstatus implementation.
type Client struct {
	// We need to embed the struct and not helmkube.Interface
	// as Helm adds more methods to the struct over time
	// without adding them to the interface. This ensures
	// we always embed all methods.
	*helmkube.Client

	newResourceManager func(sr ...NewStatusReaderFunc) *ssa.ResourceManager
	waitContext        context.Context
}

// Ensure Client implements helmkube.Interface.
var _ helmkube.Interface = (*Client)(nil)

// Ensure Client implements helmkube.InterfaceWaitOptions.
var _ helmkube.InterfaceWaitOptions = (*Client)(nil)

// NewClient returns a new Helm kube Client that uses kstatus for waits.
func NewClient(getter genericclioptions.RESTClientGetter) *Client {
	return &Client{Client: helmkube.New(getter)}
}

// GetWaiter implements helmkube.InterfaceWaitOptions by returning
// a custom kstatus-based Waiter.
func (c *Client) GetWaiter(strategy helmkube.WaitStrategy) (helmkube.Waiter, error) {
	return c.newWaiter(strategy)
}

// GetWaiterWithOptions implements helmkube.InterfaceWaitOptions by
// returning a custom kstatus-based Waiter.
func (c *Client) GetWaiterWithOptions(strategy helmkube.WaitStrategy,
	opts ...helmkube.WaitOption) (helmkube.Waiter, error) {
	return c.newWaiter(strategy, opts...)
}

// newWaiter returns a new Waiter based on the provided strategy.
func (c *Client) newWaiter(strategy helmkube.WaitStrategy,
	opts ...helmkube.WaitOption) (helmkube.Waiter, error) {

	if strategy == helmkube.LegacyStrategy || c.newResourceManager == nil {
		return c.Client.GetWaiterWithOptions(strategy, opts...)
	}

	return &waiter{
		c:                  c.Client,
		strategy:           strategy,
		newResourceManager: c.newResourceManager,
		waitContext:        c.waitContext,
	}, nil
}
