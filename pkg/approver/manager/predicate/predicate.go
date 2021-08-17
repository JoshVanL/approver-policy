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

package predicate

import (
	"context"
	"fmt"

	cmapi "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmpapi "github.com/cert-manager/policy-approver/pkg/apis/policy/v1alpha1"
	"github.com/cert-manager/policy-approver/pkg/approver/internal"
)

// Predicate is a func called by the Approver Manager to filter the set of
// CertificateRequestPolicies that should be evaluated on the
// CertificateRequest. Returned list of CertificateRequestPolicies pass the
// predicate or filter.
type Predicate func(context.Context, *cmapi.CertificateRequest, []cmpapi.CertificateRequestPolicy) ([]cmpapi.CertificateRequestPolicy, error)

// Ready is a Predicate that returns the subset of given policies that have a
// Ready condition set to True.
func Ready(_ context.Context, _ *cmapi.CertificateRequest, policies []cmpapi.CertificateRequestPolicy) ([]cmpapi.CertificateRequestPolicy, error) {
	var readyPolicies []cmpapi.CertificateRequestPolicy

	for _, crp := range policies {
		for _, condition := range crp.Status.Conditions {
			if condition.Type == cmpapi.CertificateRequestPolicyConditionReady && condition.Status == corev1.ConditionTrue {
				readyPolicies = append(readyPolicies, crp)
			}
		}
	}

	return readyPolicies, nil
}

// IssuerRefSelector is a Predicate that returns the subset of given policies
// that have an `spec.issuerRefSelector` matching the `spec.issuerRef` in the
// request.  PredicateIssuerRefSelector will match on strings using wilcards
// "*". Empty selector is equivalent to "*" and will match on anything.
func IssuerRefSelector(_ context.Context, cr *cmapi.CertificateRequest, policies []cmpapi.CertificateRequestPolicy) ([]cmpapi.CertificateRequestPolicy, error) {
	var matchingPolicies []cmpapi.CertificateRequestPolicy

	for _, crp := range policies {
		issRefSel := crp.Spec.IssuerRefSelector
		issRef := cr.Spec.IssuerRef

		if issRefSel.Name != nil && !internal.WildcardMatchs(*issRefSel.Name, issRef.Name) {
			continue
		}
		if issRefSel.Kind != nil && !internal.WildcardMatchs(*issRefSel.Kind, issRef.Kind) {
			continue
		}
		if issRefSel.Group != nil && !internal.WildcardMatchs(*issRefSel.Group, issRef.Group) {
			continue
		}
		matchingPolicies = append(matchingPolicies, crp)
	}

	return matchingPolicies, nil
}

// RBACBoundPolicies is a Predicate that returns the subset of
// CertificateRequestPolicies that have been RBAC bound to the user in the
// CertificateRequest. Achieved using SubjectAccessReviews.
func RBACBound(client client.Client) Predicate {
	return func(ctx context.Context, cr *cmapi.CertificateRequest, policies []cmpapi.CertificateRequestPolicy) ([]cmpapi.CertificateRequestPolicy, error) {
		extra := make(map[string]authzv1.ExtraValue)
		for k, v := range cr.Spec.Extra {
			extra[k] = v
		}

		var boundPolicies []cmpapi.CertificateRequestPolicy
		for _, crp := range policies {
			// Perform subject access review for this CertificateRequestPolicy
			rev := &authzv1.SubjectAccessReview{
				Spec: authzv1.SubjectAccessReviewSpec{
					User:   cr.Spec.Username,
					Groups: cr.Spec.Groups,
					Extra:  extra,
					UID:    cr.Spec.UID,

					ResourceAttributes: &authzv1.ResourceAttributes{
						Group:     "policy.cert-manager.io",
						Resource:  "certificaterequestpolicies",
						Name:      crp.Name,
						Namespace: cr.Namespace,
						Verb:      "use",
					},
				},
			}
			if err := client.Create(ctx, rev); err != nil {
				return nil, fmt.Errorf("failed to create subjectaccessreview: %w", err)
			}

			// If the user is bound to this policy then append.
			if rev.Status.Allowed {
				boundPolicies = append(boundPolicies, crp)
			}
		}

		return boundPolicies, nil
	}
}
