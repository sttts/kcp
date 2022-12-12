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

package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kcp-dev/logicalcluster/v3"
	"github.com/stretchr/testify/require"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"

	"github.com/kcp-dev/kcp/pkg/admission/helpers"
	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	tenancyv1beta1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1beta1"
	"github.com/kcp-dev/kcp/pkg/authorization"
	tenancyv1alpha1listers "github.com/kcp-dev/kcp/pkg/client/listers/tenancy/v1alpha1"
)

func createAttr(ws *tenancyv1beta1.Workspace) admission.Attributes {
	return createAttrWithUser(ws, &user.DefaultInfo{})
}

func createAttrWithUser(ws *tenancyv1beta1.Workspace, info user.Info) admission.Attributes {
	return admission.NewAttributesRecord(
		helpers.ToUnstructuredOrDie(ws),
		nil,
		tenancyv1beta1.Kind("Workspace").WithVersion("v1beta1"),
		"",
		ws.Name,
		tenancyv1beta1.Resource("workspaces").WithVersion("v1beta1"),
		"",
		admission.Create,
		&metav1.CreateOptions{},
		false,
		info,
	)
}

func updateAttr(ws, old *tenancyv1beta1.Workspace) admission.Attributes {
	return admission.NewAttributesRecord(
		helpers.ToUnstructuredOrDie(ws),
		helpers.ToUnstructuredOrDie(old),
		tenancyv1beta1.Kind("Workspace").WithVersion("v1beta1"),
		"",
		ws.Name,
		tenancyv1beta1.Resource("workspaces").WithVersion("v1beta1"),
		"",
		admission.Update,
		&metav1.CreateOptions{},
		false,
		&user.DefaultInfo{},
	)
}

func TestAdmit(t *testing.T) {
	tests := []struct {
		name           string
		types          []*tenancyv1alpha1.ClusterWorkspaceType
		thisWorkspaces []*tenancyv1alpha1.ThisWorkspace
		clusterName    logicalcluster.Name
		a              admission.Attributes
		expectedObj    runtime.Object
		wantErr        bool
	}{
		{
			name: "adds user information on create",
			types: []*tenancyv1alpha1.ClusterWorkspaceType{
				newType("root:org:foo").ClusterWorkspaceType,
			},
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org:ws")).ThisWorkspace,
			},
			clusterName: "root:org:ws",
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			}, &user.DefaultInfo{
				Name:   "someone",
				UID:    "id",
				Groups: []string{"a", "b"},
				Extra: map[string][]string{
					"one": {"1", "01"},
				},
			}),
			expectedObj: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someone","uid":"id","groups":["a","b"],"extra":{"one":["1","01"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			},
		},
		{
			name: "keep user information on create when system:masters",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org:ws")).ThisWorkspace,
			},
			clusterName: "root:org:ws",
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someoneelse","uid":"otherid","groups":["c","d"],"extra":{"two":["2","02"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "Foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			}, &user.DefaultInfo{
				Name:   "someone",
				UID:    "id",
				Groups: []string{"a", "b", "system:masters"},
				Extra: map[string][]string{
					"one": {"1", "01"},
				},
			}),
			expectedObj: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someoneelse","uid":"otherid","groups":["c","d"],"extra":{"two":["2","02"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "Foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			},
		},
		{
			name: "override user information on create when not system:masters",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org:ws")).ThisWorkspace,
			},
			clusterName: "root:org:ws",
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someoneelse","uid":"otherid","groups":["c","d"],"extra":{"two":["2","02"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "Foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			}, &user.DefaultInfo{
				Name:   "someone",
				UID:    "id",
				Groups: []string{"a", "b"},
				Extra: map[string][]string{
					"one": {"1", "01"},
				},
			}),
			expectedObj: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someone","uid":"id","groups":["a","b"],"extra":{"one":["1","01"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "Foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			},
		},
		{
			name: "copies required groups on create",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org:ws")).WithRequiredGroups("foo", "bar").ThisWorkspace,
			},
			clusterName: "root:org:ws",
			a: createAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: tenancyv1beta1.WorkspaceSpec{},
			}),
			expectedObj: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"authorization.kcp.dev/required-groups": "foo,bar",
						"experimental.tenancy.kcp.dev/owner":    "{}",
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{},
			},
		},
		{
			name: "replaces required groups on create as non-system:master",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org:ws")).WithRequiredGroups("foo", "bar").ThisWorkspace,
			},
			clusterName: "root:org:ws",
			a: createAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"authorization.kcp.dev/required-groups": "foo,abc",
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{},
			}),
			expectedObj: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"authorization.kcp.dev/required-groups": "foo,bar",
						"experimental.tenancy.kcp.dev/owner":    "{}",
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{},
			},
		},
		{
			name: "keeps required groups on create as system:master",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org:ws")).WithRequiredGroups("foo", "bar").ThisWorkspace,
			},
			clusterName: "root:org:ws",
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"authorization.kcp.dev/required-groups": "foo,abc",
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{},
			}, &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{"system:masters"},
			}),
			expectedObj: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"authorization.kcp.dev/required-groups": "foo,abc",
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &workspace{
				Handler:             admission.NewHandler(admission.Create, admission.Update),
				thisWorkspaceLister: fakeThisWorkspaceClusterLister(tt.thisWorkspaces),
			}
			ctx := request.WithCluster(context.Background(), request.Cluster{Name: tt.clusterName})
			if err := o.Admit(ctx, tt.a, nil); (err != nil) != tt.wantErr {
				t.Fatalf("Admit() error = %v, wantErr %v", err, tt.wantErr)
			} else if err == nil {
				got, ok := tt.a.GetObject().(*unstructured.Unstructured)
				require.True(t, ok, "expected unstructured, got %T", tt.a.GetObject())
				expected := helpers.ToUnstructuredOrDie(tt.expectedObj)
				if diff := cmp.Diff(expected, got); diff != "" {
					t.Fatalf("got incorrect result: %v", diff)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name           string
		thisWorkspaces []*tenancyv1alpha1.ThisWorkspace
		a              admission.Attributes
		expectedErrors []string
	}{
		{
			name: "rejects type mutations",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: updateAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			},
				&tenancyv1beta1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
					},
					Spec: tenancyv1beta1.WorkspaceSpec{
						Type: tenancyv1beta1.WorkspaceTypeReference{
							Name: "universal",
							Path: logicalcluster.NewPath("root:org"),
						},
					},
				}),
			expectedErrors: []string{"field is immutable"},
		},
		{
			name: "rejects unsetting cluster",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: updateAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{}},
				&tenancyv1beta1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
					},
					Spec: tenancyv1beta1.WorkspaceSpec{
						Type: tenancyv1beta1.WorkspaceTypeReference{
							Name: "foo",
							Path: logicalcluster.NewPath("root:org"),
						},
					},
					Status: tenancyv1beta1.WorkspaceStatus{
						Cluster: "somewhere",
					},
				}),
			expectedErrors: []string{"status.cluster cannot be unset"},
		},
		{
			name: "allows transition to ready directly when valid",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: updateAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase:        tenancyv1alpha1.WorkspacePhaseReady,
					Initializers: []tenancyv1alpha1.WorkspaceInitializer{},
					Cluster:      "somewhere",
					URL:          "https://kcp.bigcorp.com/clusters/org:test",
				},
			},
				&tenancyv1beta1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
					},
					Spec: tenancyv1beta1.WorkspaceSpec{
						Type: tenancyv1beta1.WorkspaceTypeReference{
							Name: "foo",
							Path: logicalcluster.NewPath("root:org"),
						},
					},
					Status: tenancyv1beta1.WorkspaceStatus{
						Phase:        tenancyv1alpha1.WorkspacePhaseScheduling,
						Initializers: []tenancyv1alpha1.WorkspaceInitializer{"a"},
					},
				}),
		},
		{
			name: "allows creation to ready directly when valid",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: createAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase:        tenancyv1alpha1.WorkspacePhaseReady,
					Initializers: []tenancyv1alpha1.WorkspaceInitializer{},
					Cluster:      "somewhere",
					URL:          "https://kcp.bigcorp.com/clusters/org:test",
				},
			}),
		},
		{
			name: "rejects transition to ready directly when invalid",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: updateAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase:        tenancyv1alpha1.WorkspacePhaseReady,
					Initializers: []tenancyv1alpha1.WorkspaceInitializer{},
					Cluster:      "somewhere",
				},
			},
				&tenancyv1beta1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
					},
					Spec: tenancyv1beta1.WorkspaceSpec{
						Type: tenancyv1beta1.WorkspaceTypeReference{
							Name: "foo",
							Path: logicalcluster.NewPath("root:org"),
						},
					},
					Status: tenancyv1beta1.WorkspaceStatus{
						Phase:        tenancyv1alpha1.WorkspacePhaseScheduling,
						Initializers: []tenancyv1alpha1.WorkspaceInitializer{"a"},
					},
				}),
			expectedErrors: []string{"status.URL must be set for phase Ready"},
		},
		{
			name: "ignores different resources",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: admission.NewAttributesRecord(
				&tenancyv1alpha1.ClusterWorkspaceShard{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
					},
				},
				nil,
				tenancyv1alpha1.Kind("ClusterWorkspaceShard").WithVersion("v1alpha1"),
				"",
				"test",
				tenancyv1alpha1.Resource("clusterworkspaceshards").WithVersion("v1alpha1"),
				"",
				admission.Create,
				&metav1.CreateOptions{},
				false,
				&user.DefaultInfo{},
			),
		},
		{
			name: "checks user information on create",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: map[string]string{"experimental.tenancy.kcp.dev/owner": "{}"},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			}, &user.DefaultInfo{
				Name:   "someone",
				UID:    "id",
				Groups: []string{"a", "b"},
				Extra: map[string][]string{
					"one": {"1", "01"},
				},
			}),
			expectedErrors: []string{"expected user annotation experimental.tenancy.kcp.dev/owner={\"username\":\"someone\",\"uid\":\"id\",\"groups\":[\"a\",\"b\"],\"extra\":{\"one\":[\"1\",\"01\"]}}"},
		},
		{
			name: "accept user information on create when system:masters",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someoneelse","uid":"otherid","groups":["c","d"],"extra":{"two":["2","02"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "Foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase: tenancyv1alpha1.WorkspacePhaseScheduling,
				},
			}, &user.DefaultInfo{
				Name:   "someone",
				UID:    "id",
				Groups: []string{"a", "b", "system:masters"},
				Extra: map[string][]string{
					"one": {"1", "01"},
				},
			}),
		},
		{
			name: "reject wrong user information on create when not system:masters",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).ThisWorkspace,
			},
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": `{"username":"someoneelse","uid":"otherid","groups":["c","d"],"extra":{"two":["2","02"]}}`,
					},
				},
				Spec: tenancyv1beta1.WorkspaceSpec{
					Type: tenancyv1beta1.WorkspaceTypeReference{
						Name: "Foo",
						Path: logicalcluster.NewPath("root:org"),
					},
				},
			}, &user.DefaultInfo{
				Name:   "someone",
				UID:    "id",
				Groups: []string{"a", "b"},
				Extra: map[string][]string{
					"one": {"1", "01"},
				},
			}),
			expectedErrors: []string{"expected user annotation experimental.tenancy.kcp.dev/owner={\"username\":\"someone\",\"uid\":\"id\",\"groups\":[\"a\",\"b\"],\"extra\":{\"one\":[\"1\",\"01\"]}}"},
		},
		{
			name: "rejects with wrong required groups on create as non-system:master",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).WithRequiredGroups("foo", "bar").ThisWorkspace,
			},
			a: createAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": "{}",
					},
				},
			}),
			expectedErrors: []string{"missing required groups annotation authorization.kcp.dev/required-groups=foo,bar"},
		},
		{
			name: "accepts with equal required groups on create",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).WithRequiredGroups("foo", "bar").ThisWorkspace,
			},
			a: createAttr(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"authorization.kcp.dev/required-groups": "foo,bar",
						"experimental.tenancy.kcp.dev/owner":    "{}",
					},
				},
			}),
		},
		{
			name: "accepts with wrong required groups on create as system:master",
			thisWorkspaces: []*tenancyv1alpha1.ThisWorkspace{
				newThisWorkspace(logicalcluster.NewPath("root:org")).WithRequiredGroups("foo", "bar").ThisWorkspace,
			},
			a: createAttrWithUser(&tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"experimental.tenancy.kcp.dev/owner": "{}",
					},
				},
			}, &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{"system:masters"},
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &workspace{
				Handler:             admission.NewHandler(admission.Create, admission.Update),
				thisWorkspaceLister: fakeThisWorkspaceClusterLister(tt.thisWorkspaces),
			}
			ctx := request.WithCluster(context.Background(), request.Cluster{Name: "root:org"})
			err := o.Validate(ctx, tt.a, nil)
			t.Logf("%v", err)
			wantErr := len(tt.expectedErrors) > 0
			require.Equal(t, wantErr, err != nil, "expected error: %v, got: %v", tt.expectedErrors, err)

			if err != nil {
				t.Logf("Got admission errors: %v", err)
				for _, expected := range tt.expectedErrors {
					require.Contains(t, err.Error(), expected)
				}
			}
		})
	}
}

type builder struct {
	*tenancyv1alpha1.ClusterWorkspaceType
}

func newType(qualifiedName string) builder {
	path, name := logicalcluster.NewPath(qualifiedName).Split()
	return builder{ClusterWorkspaceType: &tenancyv1alpha1.ClusterWorkspaceType{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				logicalcluster.AnnotationKey: path.String(),
			},
		},
	}}
}

type thisBuilder struct {
	*tenancyv1alpha1.ThisWorkspace
}

func newThisWorkspace(clusterName logicalcluster.Path) thisBuilder {
	return thisBuilder{ThisWorkspace: &tenancyv1alpha1.ThisWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenancyv1alpha1.ThisWorkspaceName,
			Annotations: map[string]string{
				logicalcluster.AnnotationKey: clusterName.String(),
			},
		},
	}}
}

func (b thisBuilder) WithRequiredGroups(groups ...string) thisBuilder {
	if len(groups) > 0 {
		b.ThisWorkspace.Annotations[authorization.RequiredGroupsAnnotationKey] = strings.Join(groups, ",")
	}
	return b
}

type fakeThisWorkspaceClusterLister []*tenancyv1alpha1.ThisWorkspace

func (l fakeThisWorkspaceClusterLister) List(selector labels.Selector) (ret []*tenancyv1alpha1.ThisWorkspace, err error) {
	return l, nil
}

func (l fakeThisWorkspaceClusterLister) Cluster(cluster logicalcluster.Name) tenancyv1alpha1listers.ThisWorkspaceLister {
	var perCluster []*tenancyv1alpha1.ThisWorkspace
	for _, this := range l {
		if logicalcluster.From(this) == cluster {
			perCluster = append(perCluster, this)
		}
	}
	return fakeThisWorkspaceLister(perCluster)
}

type fakeThisWorkspaceLister []*tenancyv1alpha1.ThisWorkspace

func (l fakeThisWorkspaceLister) List(selector labels.Selector) (ret []*tenancyv1alpha1.ThisWorkspace, err error) {
	return l.ListWithContext(context.Background(), selector)
}

func (l fakeThisWorkspaceLister) ListWithContext(ctx context.Context, selector labels.Selector) (ret []*tenancyv1alpha1.ThisWorkspace, err error) {
	return l, nil
}

func (l fakeThisWorkspaceLister) Get(name string) (*tenancyv1alpha1.ThisWorkspace, error) {
	return l.GetWithContext(context.Background(), name)
}

func (l fakeThisWorkspaceLister) GetWithContext(ctx context.Context, name string) (*tenancyv1alpha1.ThisWorkspace, error) {
	for _, t := range l {
		if t.Name == name {
			return t, nil
		}
	}
	return nil, apierrors.NewNotFound(tenancyv1alpha1.Resource("clusterworkspace"), name)
}
