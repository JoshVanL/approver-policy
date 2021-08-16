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

package attribute

import (
	"context"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/cert-manager/policy-approver/pkg/approver"
	"github.com/cert-manager/policy-approver/pkg/registry"
)

// Load the attribute evaluator checks.
func init() {
	registry.Shared.Store(attribute{})
}

var _ approver.Interface = attribute{}

// attribute is the "default" Approver that is responsible for the base fields
// on the CertificateRequestPolicy. It is expected that attribute must _always_
// be registered for all policy-approvers.
type attribute struct {
	registeredPlugins []string
}

// Name of Approver is "attribute"
func (a attribute) Name() string {
	return "attribute"
}

// RegisterFlags is a no-op, attribute doesn't need any flags.
func (a attribute) RegisterFlags(_ *pflag.FlagSet) {
	return
}

// Prepare will collect the list of registered plugins to be used in
// validation.
func (a attribute) Prepare(_ context.Context, _ manager.Manager) error {
	for _, approver := range registry.Shared.Approvers() {
		// Don't allow plugins with the same name as the base attribute approver.
		if approver.Name() != a.Name() {
			a.registeredPlugins = append(a.registeredPlugins, approver.Name())
		}
	}
	return nil
}
