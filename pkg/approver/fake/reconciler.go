/*
Copyright 2021 The cert-manager Authors.

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

package fake

import (
	"context"

	cmpapi "github.com/cert-manager/policy-approver/pkg/apis/policy/v1alpha1"
	"github.com/cert-manager/policy-approver/pkg/approver"
)

var _ approver.Reconciler = &FakeReconciler{}

// FakeReconciler is a testing reconciler designed to mock Reconcilers with a
// pre-determined response.
type FakeReconciler struct {
	readyFunc func(context.Context, *cmpapi.CertificateRequestPolicy) (approver.ReconcilerReadyResponse, error)
}

func NewFakeReconciler() *FakeReconciler {
	return new(FakeReconciler)
}

func (f *FakeReconciler) WithReady(fn func(context.Context, *cmpapi.CertificateRequestPolicy) (approver.ReconcilerReadyResponse, error)) *FakeReconciler {
	f.readyFunc = fn
	return f
}

func (f *FakeReconciler) Ready(ctx context.Context, crp *cmpapi.CertificateRequestPolicy) (approver.ReconcilerReadyResponse, error) {
	return f.readyFunc(ctx, crp)
}
