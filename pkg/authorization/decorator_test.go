/*
Copyright 2022 The KCP Authors.

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

package authorization

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	auditapis "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func TestDecorator(t *testing.T) {
	alwaysAllow := authorizer.AuthorizerFunc(func(ctx context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
		return authorizer.DecisionAllow, "unanonymized allow", nil
	})
	alwaysDeny := authorizer.AuthorizerFunc(func(ctx context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
		return authorizer.DecisionDeny, "unanonymized denial", nil
	})
	alwaysNoOpinion := authorizer.AuthorizerFunc(func(ctx context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
		return authorizer.DecisionNoOpinion, "unanonymized no-opinion", nil
	})
	alwaysError := authorizer.AuthorizerFunc(func(ctx context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
		return authorizer.DecisionNoOpinion, "unanonymized failure", errors.New("unanonymized error")
	})

	for name, tc := range map[string]struct {
		authz        authorizer.Authorizer
		wantDecision authorizer.Decision
		wantAudit    map[string]string
		wantReason   string
	}{
		"topAllows": {
			authz: WithAuditLogging("domain", NewDecorator("top", alwaysAllow).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "top: access granted",

			wantAudit: map[string]string{
				"top.domain/decision": "Allowed",
				"top.domain/reason":   "unanonymized allow",
			},
		},
		"topAllowsWithoutAudit": {
			authz: NewDecorator("top", alwaysAllow).AddAuditLogging().AddAnonymization().AddReasonAnnotation(),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "top: access granted",

			wantAudit: nil,
		},
		"topAllowsWithoutReasonAnnotation": {
			authz: WithAuditLogging("domain", NewDecorator("top", alwaysAllow).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "access granted",

			wantAudit: map[string]string{
				"top.domain/decision": "Allowed",
				"top.domain/reason":   "unanonymized allow",
			},
		},
		"topAllowsWithoutReasonAnnotationWithoutAnonymization": {
			authz: WithAuditLogging("domain", NewDecorator("top", alwaysAllow).AddAuditLogging()),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "unanonymized allow",

			wantAudit: map[string]string{
				"top.domain/decision": "Allowed",
				"top.domain/reason":   "unanonymized allow",
			},
		},
		"topAllowsWithoutReasonAnnotationWithoutAnonymizationWithoutAuditLogging": {
			authz: WithAuditLogging("domain", NewDecorator("top", alwaysAllow)),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "unanonymized allow",

			wantAudit: nil,
		},
		"topDelegatesToAllow": {
			authz: WithAuditLogging("domain", NewDecorator("top",
				DelegateAuthorization("top-to-bottom",
					NewDecorator("bottom", alwaysAllow).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
			).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "access granted",

			wantAudit: map[string]string{
				"bottom.domain/decision": "Allowed",
				"bottom.domain/reason":   "unanonymized allow",

				"top.domain/decision": "Allowed",
				"top.domain/reason":   "delegating due to top-to-bottom: bottom: access granted",
			},
		},
		"topDelegatesToDeny": {
			authz: WithAuditLogging("domain", NewDecorator("top",
				DelegateAuthorization("top-to-bottom",
					NewDecorator("bottom", alwaysDeny).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
			).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionDeny,
			wantReason:   "access denied",

			wantAudit: map[string]string{
				"bottom.domain/decision": "Denied",
				"bottom.domain/reason":   "unanonymized denial",

				"top.domain/decision": "Denied",
				"top.domain/reason":   "delegating due to top-to-bottom: bottom: access denied",
			},
		},
		"topDelegatesToDelegateDelegatesToDeny": {
			authz: WithAuditLogging("domain", NewDecorator("top",
				DelegateAuthorization("top-to-middle", NewDecorator("middle",
					DelegateAuthorization("middle-to-bottom", NewDecorator("bottom", alwaysDeny).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
				).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
			).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionDeny,
			wantReason:   "access denied",

			wantAudit: map[string]string{
				"bottom.domain/decision": "Denied",
				"bottom.domain/reason":   "unanonymized denial",

				"middle.domain/decision": "Denied",
				"middle.domain/reason":   "delegating due to middle-to-bottom: bottom: access denied",

				"top.domain/decision": "Denied",
				"top.domain/reason":   "delegating due to top-to-middle: middle: access denied",
			},
		},
		"topDelegatesToDelegateDelegatesToAllow": {
			authz: WithAuditLogging("domain", NewDecorator("top",
				DelegateAuthorization("top-to-middle", NewDecorator("middle",
					DelegateAuthorization("middle-to-bottom", NewDecorator("bottom", alwaysAllow).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
				).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
			).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionAllow,
			wantReason:   "access granted",

			wantAudit: map[string]string{
				"bottom.domain/decision": "Allowed",
				"bottom.domain/reason":   "unanonymized allow",

				"middle.domain/decision": "Allowed",
				"middle.domain/reason":   "delegating due to middle-to-bottom: bottom: access granted",

				"top.domain/decision": "Allowed",
				"top.domain/reason":   "delegating due to top-to-middle: middle: access granted",
			},
		},
		"topDelegatesToDelegateDelegatesToNoOpinion": {
			authz: WithAuditLogging("domain", NewDecorator("top",
				DelegateAuthorization("top-to-middle", NewDecorator("middle",
					DelegateAuthorization("middle-to-bottom", NewDecorator("bottom", alwaysNoOpinion).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
				).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
			).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionNoOpinion,
			wantReason:   "access denied",

			wantAudit: map[string]string{
				"bottom.domain/decision": "NoOpinion",
				"bottom.domain/reason":   "unanonymized no-opinion",

				"middle.domain/decision": "NoOpinion",
				"middle.domain/reason":   "delegating due to middle-to-bottom: bottom: access denied",

				"top.domain/decision": "NoOpinion",
				"top.domain/reason":   "delegating due to top-to-middle: middle: access denied",
			},
		},
		"topDelegatesToDelegateDelegatesToError": {
			authz: WithAuditLogging("domain", NewDecorator("top",
				DelegateAuthorization("top-to-middle", NewDecorator("middle",
					DelegateAuthorization("middle-to-bottom", NewDecorator("bottom", alwaysError).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
				).AddAuditLogging().AddAnonymization().AddReasonAnnotation()),
			).AddAuditLogging().AddAnonymization()),

			wantDecision: authorizer.DecisionNoOpinion,
			wantReason:   "access denied",

			wantAudit: map[string]string{
				"bottom.domain/decision": "NoOpinion",
				"bottom.domain/reason":   "reason: unanonymized failure, error: unanonymized error",

				"middle.domain/decision": "NoOpinion",
				"middle.domain/reason":   "reason: delegating due to middle-to-bottom: bottom: access denied, error: unanonymized error",

				"top.domain/decision": "NoOpinion",
				"top.domain/reason":   "reason: delegating due to top-to-middle: middle: access denied, error: unanonymized error",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := audit.WithAuditContext(context.Background(), newAuditContext(auditapis.LevelMetadata))
			attr := authorizer.AttributesRecord{}
			dec, reason, _ := tc.authz.Authorize(ctx, attr)
			if dec != tc.wantDecision {
				t.Errorf("want decision %v got %v", tc.wantDecision, dec)
			}
			ev := audit.AuditEventFrom(ctx)
			if diff := cmp.Diff(tc.wantAudit, ev.Annotations); diff != "" {
				t.Errorf("audit log annotations differ: %v", diff)
			}
			if tc.wantReason != reason {
				t.Errorf("want reason %q, got %q", tc.wantReason, reason)
			}
		})
	}
}

func newAuditContext(l auditapis.Level) *audit.AuditContext {
	return &audit.AuditContext{
		Event: &auditapis.Event{
			Level: l,
		},
	}
}
