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

package manager

import (
	"context"
	"fmt"
	"sort"
	"strings"

	cmapi "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmpapi "github.com/cert-manager/policy-approver/pkg/apis/policy/v1alpha1"
	"github.com/cert-manager/policy-approver/pkg/approver"
	"github.com/cert-manager/policy-approver/pkg/approver/internal"
)

var _ Interface = &subjectaccessreview{}

// subjectaccessreview is a manager that calls evaluators with
// CertificateRequestPolicys that have been RBAC bound to the user who appears
// in the passed CertificateRequest to Evaluate.
type subjectaccessreview struct {
	client client.Client

	evaluators []approver.Evaluator
}

// NewSubjectAccessReview constructs a new approver Manager which evaluates
// whether CertificateRequests should be approved or denied, managing
// registered evaluators.
func NewSubjectAccessReview(client client.Client, evaluators []approver.Evaluator) *subjectaccessreview {
	return &subjectaccessreview{
		client:     client,
		evaluators: evaluators,
	}
}

// Review will evaluate whether the incoming CertificateRequest should be
// approved. All evaluators will be called with CertificateRequestPolicys that
// have been RBAC bound to the user included in the CertificateRequest.
func (s *subjectaccessreview) Review(ctx context.Context, cr *cmapi.CertificateRequest) (ReviewResponse, error) {
	crps := new(cmpapi.CertificateRequestPolicyList)
	if err := s.client.List(ctx, crps); err != nil {
		return ReviewResponse{}, err
	}

	// If no CertificateRequestPolicies exist in the cluster, return
	// ResultUnprocessed.
	if len(crps.Items) == 0 {
		return ReviewResponse{Result: ResultUnprocessed, Message: "No CertificateRequestPolicies exist"}, nil
	}

	policies := issuerRefSelector(cr, crps.Items)
	policies, err := s.boundPolicies(ctx, cr, policies)
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("failed to determine bound policies: %w", err)
	}

	// If no policies are appropriate, return unprocessed.
	if len(policies) == 0 {
		return ReviewResponse{
			Result:  ResultUnprocessed,
			Message: "No CertificateRequestPolicies bound or applicable",
		}, nil
	}

	// policyMessages hold the aggregated messages of each evaluator response,
	// keyed by the policy name that was executed.
	var policyMessages []policyMessage

	// Run every evaluators against ever policy which is bound to the requesting
	// user.
	for _, crp := range policies {
		var (
			evaluatorDenied   bool
			evaluatorMessages []string
		)

		for _, evaluator := range s.evaluators {
			response, err := evaluator.Evaluate(ctx, &crp, cr)
			if err != nil {
				// if a single evaluator errors, then return early without trying
				// others.
				return ReviewResponse{}, err
			}

			evaluatorMessages = append(evaluatorMessages, response.Message)

			// evaluatorDenied will be set to true if any evaluator denies. We don't
			// break early so that we can capture the responses from _all_
			// evaluators.
			if response.Result == approver.ResultDenied {
				evaluatorDenied = true
			}
		}

		// If no evaluator denied the request, return with approved response.
		if !evaluatorDenied {
			return ReviewResponse{
				Result:  ResultApproved,
				Message: fmt.Sprintf("Approved by CertificateRequestPolicy: %q", crp.Name),
			}, nil
		}

		// Collect evaluator messages that were executed for this policy.
		policyMessages = append(policyMessages, policyMessage{name: crp.Name, message: strings.Join(evaluatorMessages, ", ")})
	}

	// Sort messages by policy name and build message string.
	sort.SliceStable(policyMessages, func(i, j int) bool {
		return policyMessages[i].name < policyMessages[j].name
	})
	var messages []string
	for _, policyMessage := range policyMessages {
		messages = append(messages, fmt.Sprintf("[%s: %s]", policyMessage.name, policyMessage.message))
	}

	// Return with all policies that we consulted, and their errors to why the
	// request was denied.
	return ReviewResponse{
		Result:  ResultDenied,
		Message: fmt.Sprintf("No policy approved this request: %s", strings.Join(messages, " ")),
	}, nil
}

// issuerRefSelector returns the subset of given policies that have an
// `spec.issuerRefSelector` matching the `spec.issuerRef` in the request.
// issuerRefSelector will match on strings using wilcards "*". Empty selector
// is equivalent to "*" and will match on anything.
func issuerRefSelector(cr *cmapi.CertificateRequest, allPolicies []cmpapi.CertificateRequestPolicy) []cmpapi.CertificateRequestPolicy {
	var matchingPolicies []cmpapi.CertificateRequestPolicy

	for _, crp := range allPolicies {
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

	return matchingPolicies
}

// boundPolicies returns the subset of given policies which are RBAC bound to
// the user defined in the request.
func (s *subjectaccessreview) boundPolicies(ctx context.Context, cr *cmapi.CertificateRequest, allPolicies []cmpapi.CertificateRequestPolicy) ([]cmpapi.CertificateRequestPolicy, error) {
	extra := make(map[string]authzv1.ExtraValue)
	for k, v := range cr.Spec.Extra {
		extra[k] = v
	}

	var boundPolicies []cmpapi.CertificateRequestPolicy
	for _, crp := range allPolicies {
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
		if err := s.client.Create(ctx, rev); err != nil {
			return nil, fmt.Errorf("failed to create subjectaccessreview: %w", err)
		}

		// If the user is bound to this policy then append.
		if rev.Status.Allowed {
			boundPolicies = append(boundPolicies, crp)
		}
	}

	return boundPolicies, nil
}
