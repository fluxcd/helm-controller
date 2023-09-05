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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	kuberecorder "k8s.io/client-go/tools/record"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	internalController "github.com/fluxcd/helm-controller/internal/controller"
	runtimeClient "github.com/fluxcd/pkg/runtime/client"
	helper "github.com/fluxcd/pkg/runtime/controller"
)

type HelmReleaseReconcilerFactory struct {
	client.Client
	helper.Metrics

	Config                *rest.Config
	Scheme                *runtime.Scheme
	EventRecorder         kuberecorder.EventRecorder
	DefaultServiceAccount string
	NoCrossNamespaceRef   bool
	ClientOpts            runtimeClient.Options
	KubeConfigOpts        runtimeClient.KubeConfigOptions
	StatusPoller          *polling.StatusPoller
	PollingOpts           polling.Options
	ControllerName        string
}

type HelmReleaseReconcilerOptions internalController.HelmReleaseReconcilerOptions

func (f *HelmReleaseReconcilerFactory) SetupWithManager(ctx context.Context, mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	r := &internalController.HelmReleaseReconciler{
		Client:                f.Client,
		Metrics:               f.Metrics,
		Config:                f.Config,
		Scheme:                f.Scheme,
		EventRecorder:         f.EventRecorder,
		DefaultServiceAccount: f.DefaultServiceAccount,
		NoCrossNamespaceRef:   f.NoCrossNamespaceRef,
		ClientOpts:            f.ClientOpts,
		KubeConfigOpts:        f.KubeConfigOpts,
		StatusPoller:          f.StatusPoller,
		PollingOpts:           f.PollingOpts,
		ControllerName:        f.ControllerName,
	}
	return r.SetupWithManager(ctx, mgr, internalController.HelmReleaseReconcilerOptions(opts))
}
