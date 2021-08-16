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

package webhook

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/cert-manager/policy-approver/pkg/approver"
)

// Options are options for running the wehook.
type Options struct {
	// Log is a shared logger for the shared webhook.
	Log logr.Logger

	// Webhooks is the list of registered Webhooks that will be used to build the
	// shared webhook server.
	Webhooks []approver.Webhook
}

// Register the policy-approver Webhook endpoints against the
// controller-manager Manager.
func Register(mgr manager.Manager, opts Options) {
	opts.Log.Info("registering webhook endpoints")

	validator := &validator{
		log:      opts.Log.WithName("validation"),
		lister:   mgr.GetCache(),
		webhooks: opts.Webhooks,
	}
	mgr.GetWebhookServer().Register("/validate", &webhook.Admission{Handler: validator})
	mgr.AddReadyzCheck("validator", validator.check)
}
