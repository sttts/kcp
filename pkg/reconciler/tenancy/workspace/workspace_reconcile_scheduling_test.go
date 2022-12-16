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
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	kcpcache "github.com/kcp-dev/apimachinery/v2/pkg/cache"
	kcpkubernetesclientset "github.com/kcp-dev/client-go/kubernetes"
	kcpfakekubeclient "github.com/kcp-dev/client-go/kubernetes/fake"
	"github.com/kcp-dev/logicalcluster/v3"
	"github.com/martinlindhe/base36"

	kcpclientgotesting "github.com/kcp-dev/client-go/third_party/k8s.io/client-go/testing"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/kcp-dev/kcp/pkg/admission/clusterworkspacetypeexists"
	"github.com/kcp-dev/kcp/pkg/apis/core"
	corev1alpha1 "github.com/kcp-dev/kcp/pkg/apis/core/v1alpha1"
	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	tenancyv1beta1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1beta1"
	conditionsapi "github.com/kcp-dev/kcp/pkg/apis/third_party/conditions/apis/conditions/v1alpha1"
	kcpclientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/cluster"
	kcpfakeclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/cluster/fake"
	"github.com/kcp-dev/kcp/pkg/indexers"
	"github.com/kcp-dev/kcp/pkg/reconciler/tenancy/workspacedeletion/deletion"
)

func TestReconcileScheduling(t *testing.T) {
	scenarios := []struct {
		name                         string
		initialShards                []*tenancyv1alpha1.ClusterWorkspaceShard
		initialClusterWorkspaceTypes []*tenancyv1alpha1.ClusterWorkspaceType
		initialKubeClientObjects     []runtime.Object
		initialKcpClientObjects      []runtime.Object
		targetWorkspace              *tenancyv1beta1.Workspace
		targetLogicalCluster         *corev1alpha1.LogicalCluster
		validateWorkspace            func(t *testing.T, initialWS, ws *tenancyv1beta1.Workspace)
		validateKubeClientActions    func(t *testing.T, a []kcpclientgotesting.Action)
		validateKcpClientActions     func(t *testing.T, a []kcpclientgotesting.Action)
		expectedKubeClientActions    []string
		expectedKcpClientActions     []string
		expectedStatus               reconcileStatus
	}{
		{
			name:                 "two-phase commit, part one: a new workspace gets a shard assigned",
			initialShards:        []*tenancyv1alpha1.ClusterWorkspaceShard{shard("root")},
			targetWorkspace:      workspace("foo"),
			targetLogicalCluster: &corev1alpha1.LogicalCluster{},
			validateWorkspace: func(t *testing.T, initialWS, ws *tenancyv1beta1.Workspace) {
				initialWS.Annotations["internal.tenancy.kcp.dev/cluster"] = "root-foo"
				initialWS.Annotations["internal.tenancy.kcp.dev/shard"] = "1pfxsevk"
				initialWS.Finalizers = append(initialWS.Finalizers, "core.kcp.dev/logicalcluster")
				if !equality.Semantic.DeepEqual(ws, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(ws, initialWS)))
				}
			},
			expectedStatus: reconcileStatusStopAndRequeue,
		},
		{
			name:                         "two-phase commit, part two: location is set",
			initialShards:                []*tenancyv1alpha1.ClusterWorkspaceShard{shard("root")},
			initialClusterWorkspaceTypes: wellKnownClusterWorkspaceTypes(),
			targetWorkspace:              wellKnownFooWSForPhaseTwo(),
			targetLogicalCluster:         &corev1alpha1.LogicalCluster{},
			validateWorkspace: func(t *testing.T, initialWS, wsAfterReconciliation *tenancyv1beta1.Workspace) {
				clearLastTransitionTimeOnWsConditions(wsAfterReconciliation)
				initialWS.CreationTimestamp = wsAfterReconciliation.CreationTimestamp
				initialWS.Status.URL = `https://root/clusters/root-foo`
				initialWS.Status.Cluster = "root-foo"
				initialWS.Status.Conditions = append(initialWS.Status.Conditions, conditionsapi.Condition{
					Type:   tenancyv1alpha1.WorkspaceScheduled,
					Status: corev1.ConditionTrue,
				})
				if !equality.Semantic.DeepEqual(wsAfterReconciliation, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(wsAfterReconciliation, initialWS)))
				}
			},
			validateKubeClientActions: func(t *testing.T, actions []kcpclientgotesting.Action) {
				validateWellKnownCRBCreationAction(t, actions)
			},
			validateKcpClientActions: func(t *testing.T, actions []kcpclientgotesting.Action) {
				validateWellKnownLogicalClusterActions(t, actions)
			},
			expectedStatus:            reconcileStatusContinue,
			expectedKubeClientActions: []string{"create:clusterrolebindings"},
			expectedKcpClientActions:  []string{"create:logicalclusters", "get:logicalclusters", "update:logicalclusters"},
		},
		{
			name:                         "two-phase commit, part two failure: LogicalCluster already exists with the right owner",
			initialShards:                []*tenancyv1alpha1.ClusterWorkspaceShard{shard("root")},
			initialClusterWorkspaceTypes: wellKnownClusterWorkspaceTypes(),
			initialKcpClientObjects: []runtime.Object{func() runtime.Object {
				thisWS := wellKnownLogicalClusterForFooWS()
				thisWS.Annotations["kcp.dev/cluster"] = "root-foo"
				return thisWS
			}()},
			targetWorkspace:      wellKnownFooWSForPhaseTwo(),
			targetLogicalCluster: &corev1alpha1.LogicalCluster{},
			validateWorkspace: func(t *testing.T, initialWS, wsAfterReconciliation *tenancyv1beta1.Workspace) {
				clearLastTransitionTimeOnWsConditions(wsAfterReconciliation)
				initialWS.CreationTimestamp = wsAfterReconciliation.CreationTimestamp
				initialWS.Status.URL = `https://root/clusters/root-foo`
				initialWS.Status.Cluster = "root-foo"
				initialWS.Status.Conditions = append(initialWS.Status.Conditions, conditionsapi.Condition{
					Type:   tenancyv1alpha1.WorkspaceScheduled,
					Status: corev1.ConditionTrue,
				})
				if !equality.Semantic.DeepEqual(wsAfterReconciliation, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(wsAfterReconciliation, initialWS)))
				}
			},
			validateKubeClientActions: func(t *testing.T, actions []kcpclientgotesting.Action) {
				validateWellKnownCRBCreationAction(t, actions)
			},
			validateKcpClientActions: func(t *testing.T, actions []kcpclientgotesting.Action) {
				validateWellKnownLogicalClusterActions(t, actions)
			},
			expectedStatus:            reconcileStatusContinue,
			expectedKubeClientActions: []string{"create:clusterrolebindings"},
			expectedKcpClientActions:  []string{"create:logicalclusters", "get:logicalclusters", "get:logicalclusters", "update:logicalclusters"},
		},
		{
			name:                         "two-phase commit, part two failure: LogicalCluster already exists with the wrong owner",
			initialShards:                []*tenancyv1alpha1.ClusterWorkspaceShard{shard("root")},
			initialClusterWorkspaceTypes: wellKnownClusterWorkspaceTypes(),
			initialKcpClientObjects: []runtime.Object{func() runtime.Object {
				thisWS := wellKnownLogicalClusterForFooWS()
				thisWS.Annotations["kcp.dev/cluster"] = "root-foo"
				thisWS.Spec.Owner.UID = "wrong-uid"
				return thisWS
			}()},
			targetWorkspace:      wellKnownFooWSForPhaseTwo(),
			targetLogicalCluster: &corev1alpha1.LogicalCluster{},
			validateWorkspace: func(t *testing.T, initialWS, wsAfterReconciliation *tenancyv1beta1.Workspace) {
				clearLastTransitionTimeOnWsConditions(wsAfterReconciliation)
				initialWS.CreationTimestamp = wsAfterReconciliation.CreationTimestamp
				delete(initialWS.Annotations, "internal.tenancy.kcp.dev/cluster")
				if !equality.Semantic.DeepEqual(wsAfterReconciliation, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(wsAfterReconciliation, initialWS)))
				}
			},
			expectedStatus:           reconcileStatusStopAndRequeue,
			expectedKcpClientActions: []string{"create:logicalclusters", "get:logicalclusters"},
		},
		{
			name:                         "two-phase commit, part two failure: CRB, LogicalCluster already exists",
			initialShards:                []*tenancyv1alpha1.ClusterWorkspaceShard{shard("root")},
			initialClusterWorkspaceTypes: wellKnownClusterWorkspaceTypes(),
			initialKubeClientObjects: []runtime.Object{func() runtime.Object {
				crb := wellKnownCRBForThisWS()
				crb.Annotations["kcp.dev/cluster"] = "root-foo"
				return crb
			}()},
			initialKcpClientObjects: []runtime.Object{func() runtime.Object {
				thisWS := wellKnownLogicalClusterForFooWS()
				thisWS.Annotations["kcp.dev/cluster"] = "root-foo"
				return thisWS
			}()},
			targetWorkspace:      wellKnownFooWSForPhaseTwo(),
			targetLogicalCluster: &corev1alpha1.LogicalCluster{},
			validateWorkspace: func(t *testing.T, initialWS, wsAfterReconciliation *tenancyv1beta1.Workspace) {
				clearLastTransitionTimeOnWsConditions(wsAfterReconciliation)
				initialWS.CreationTimestamp = wsAfterReconciliation.CreationTimestamp
				initialWS.Status.URL = `https://root/clusters/root-foo`
				initialWS.Status.Cluster = "root-foo"
				initialWS.Status.Conditions = append(initialWS.Status.Conditions, conditionsapi.Condition{
					Type:   tenancyv1alpha1.WorkspaceScheduled,
					Status: corev1.ConditionTrue,
				})
				if !equality.Semantic.DeepEqual(wsAfterReconciliation, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(wsAfterReconciliation, initialWS)))
				}
			},
			validateKubeClientActions: func(t *testing.T, actions []kcpclientgotesting.Action) {
				validateWellKnownCRBCreationAction(t, actions)
			},
			validateKcpClientActions: func(t *testing.T, actions []kcpclientgotesting.Action) {
				validateWellKnownLogicalClusterActions(t, actions)
			},
			expectedStatus:            reconcileStatusContinue,
			expectedKubeClientActions: []string{"create:clusterrolebindings"},
			expectedKcpClientActions:  []string{"create:logicalclusters", "get:logicalclusters", "get:logicalclusters", "update:logicalclusters"},
		},
		{
			name:                 "no shards available, the ws is unscheduled",
			targetWorkspace:      workspace("foo"),
			targetLogicalCluster: &corev1alpha1.LogicalCluster{},
			validateWorkspace: func(t *testing.T, initialWS, wsAfterReconciliation *tenancyv1beta1.Workspace) {
				clearLastTransitionTimeOnWsConditions(wsAfterReconciliation)
				initialWS.Status.Conditions = append(initialWS.Status.Conditions, conditionsapi.Condition{
					Type:     tenancyv1alpha1.WorkspaceScheduled,
					Severity: conditionsapi.ConditionSeverityError,
					Status:   corev1.ConditionFalse,
					Reason:   tenancyv1alpha1.WorkspaceReasonUnschedulable,
					Message:  "No available shards to schedule the workspace.",
				})
				if !equality.Semantic.DeepEqual(wsAfterReconciliation, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(wsAfterReconciliation, initialWS)))
				}
			},
			expectedStatus: reconcileStatusContinue,
		},
		{
			name: "the ws is scheduled onto requested shard (shard name in spec)",
			targetWorkspace: func() *tenancyv1beta1.Workspace {
				ws := workspace("foo")
				selector := &metav1.LabelSelector{MatchLabels: map[string]string{"awesome.shard": "amber"}}
				ws.Spec.Location.Selector = selector
				return ws
			}(),
			targetLogicalCluster: &corev1alpha1.LogicalCluster{},
			initialShards: []*tenancyv1alpha1.ClusterWorkspaceShard{shard("root"), func() *tenancyv1alpha1.ClusterWorkspaceShard {
				s := shard("amber")
				s.Labels["awesome.shard"] = "amber"
				return s
			}()},
			validateWorkspace: func(t *testing.T, initialWS, wsAfterReconciliation *tenancyv1beta1.Workspace) {
				initialWS.Annotations["internal.tenancy.kcp.dev/cluster"] = "root-foo"
				initialWS.Annotations["internal.tenancy.kcp.dev/shard"] = "29hdqnv7"
				initialWS.Finalizers = append(initialWS.Finalizers, "core.kcp.dev/logicalcluster")
				if !equality.Semantic.DeepEqual(wsAfterReconciliation, initialWS) {
					t.Fatal(fmt.Errorf("unexpected Workspace:\n%s", cmp.Diff(wsAfterReconciliation, initialWS)))
				}
			},
			expectedStatus: reconcileStatusStopAndRequeue,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			fakeKubeClient := kcpfakekubeclient.NewSimpleClientset(scenario.initialKubeClientObjects...)
			fakeKcpClient := kcpfakeclient.NewSimpleClientset(scenario.initialKcpClientObjects...)

			clusterWorkspaceTypeIndexer := cache.NewIndexer(kcpcache.MetaClusterNamespaceKeyFunc, cache.Indexers{})
			indexers.AddIfNotPresentOrDie(clusterWorkspaceTypeIndexer, cache.Indexers{
				indexers.ByLogicalClusterPathAndName: indexers.IndexByLogicalClusterPathAndName,
			})
			for _, obj := range scenario.initialClusterWorkspaceTypes {
				if err := clusterWorkspaceTypeIndexer.Add(obj); err != nil {
					t.Error(err)
				}
			}
			getType := func(path logicalcluster.Path, name string) (*tenancyv1alpha1.ClusterWorkspaceType, error) {
				objs, err := clusterWorkspaceTypeIndexer.ByIndex(indexers.ByLogicalClusterPathAndName, path.Join(name).String())
				if err != nil {
					return nil, err
				}
				if len(objs) == 0 {
					return nil, fmt.Errorf("no ClusterWorkspaceType found for %s", path.Join(name).String())
				}
				if len(objs) > 1 {
					return nil, fmt.Errorf("multiple ClusterWorkspaceTypes found for %s", path.Join(name).String())
				}
				return objs[0].(*tenancyv1alpha1.ClusterWorkspaceType), nil
			}

			target := schedulingReconciler{
				generateClusterName: func(path logicalcluster.Path) logicalcluster.Name {
					return logicalcluster.Name(strings.ReplaceAll(path.String(), ":", "-"))
				},
				kubeLogicalClusterAdminClientFor: func(shard *tenancyv1alpha1.ClusterWorkspaceShard) (kcpkubernetesclientset.ClusterInterface, error) {
					return fakeKubeClient, nil
				},
				kcpLogicalClusterAdminClientFor: func(shard *tenancyv1alpha1.ClusterWorkspaceShard) (kcpclientset.ClusterInterface, error) {
					return fakeKcpClient, nil
				},
				getShard: func(name string) (*tenancyv1alpha1.ClusterWorkspaceShard, error) {
					for _, shard := range scenario.initialShards {
						if shard.Name == name {
							return shard, nil
						}
					}
					return nil, kerrors.NewNotFound(tenancyv1alpha1.Resource("clusterworkspaceshard"), name)
				},
				listShards: func(selector labels.Selector) ([]*tenancyv1alpha1.ClusterWorkspaceShard, error) {
					var shards []*tenancyv1alpha1.ClusterWorkspaceShard
					for _, shard := range scenario.initialShards {
						if selector.Matches(labels.Set(shard.Labels)) {
							shards = append(shards, shard)
						}
					}
					return shards, nil
				},
				getShardByHash: func(hash string) (*tenancyv1alpha1.ClusterWorkspaceShard, error) {
					for _, shard := range scenario.initialShards {
						if shardNameToBase36Sha224(shard.Name) == hash {
							return shard, nil
						}
					}
					return nil, kerrors.NewNotFound(tenancyv1alpha1.SchemeGroupVersion.WithResource("ClusterWorkspaceShard").GroupResource(), hash)
				},
				getClusterWorkspaceType: getType,
				getLogicalCluster: func(clusterName logicalcluster.Name) (*corev1alpha1.LogicalCluster, error) {
					if clusterName != logicalcluster.Name("root") {
						return nil, fmt.Errorf("unexpected cluster name = %v, expected = %v", clusterName, "root")
					}
					if scenario.targetLogicalCluster == nil {
						return nil, fmt.Errorf("targetLogicalCluster wasn't provided for this scenario")
					}
					return scenario.targetLogicalCluster, nil
				},
				transitiveTypeResolver: clusterworkspacetypeexists.NewTransitiveTypeResolver(getType),
			}
			targetWorkspaceCopy := scenario.targetWorkspace.DeepCopy()
			status, err := target.reconcile(context.TODO(), scenario.targetWorkspace)
			if err != nil {
				t.Fatal(err)
			}
			if status != scenario.expectedStatus {
				t.Fatalf("unexpected reconciliation status:%v, expected:%v", status, scenario.expectedStatus)
			}
			if err := validateActionsVerbs(fakeKubeClient.Actions(), scenario.expectedKubeClientActions); err != nil {
				t.Fatalf("incorrect action(s) for kube client: %v", err)
			}
			if err := validateActionsVerbs(fakeKcpClient.Actions(), scenario.expectedKcpClientActions); err != nil {
				t.Fatalf("incorrect action(s) for kcp client: %v", err)
			}
			if scenario.validateWorkspace != nil {
				scenario.validateWorkspace(t, targetWorkspaceCopy, scenario.targetWorkspace)
			}
			if scenario.validateKubeClientActions != nil {
				scenario.validateKubeClientActions(t, fakeKubeClient.Actions())
			}
			if scenario.validateKcpClientActions != nil {
				scenario.validateKcpClientActions(t, fakeKcpClient.Actions())
			}
		})
	}
}

func workspace(name string) *tenancyv1beta1.Workspace {
	return &tenancyv1beta1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{"kcp.dev/cluster": "root"},
		},
		Spec: tenancyv1beta1.WorkspaceSpec{
			Location: &tenancyv1beta1.WorkspaceLocation{},
		},
		Status: tenancyv1beta1.WorkspaceStatus{
			Phase: corev1alpha1.LogicalClusterPhaseScheduling,
		},
	}
}

func wellKnownFooWSForPhaseTwo() *tenancyv1beta1.Workspace {
	ws := workspace("foo")
	// since this is part two we can assume the following fields are assigned
	ws.Annotations["internal.tenancy.kcp.dev/cluster"] = "root-foo"
	ws.Annotations["internal.tenancy.kcp.dev/shard"] = "1pfxsevk"
	ws.Annotations["experimental.tenancy.kcp.dev/owner"] = `{"username":"kcp-admin"}`
	ws.Finalizers = append(ws.Finalizers, "core.kcp.dev/logicalcluster")
	// type info is assigned by an admission plugin
	ws.Spec.Type = tenancyv1beta1.WorkspaceTypeReference{
		Name: "universal",
		Path: "root",
	}
	return ws
}

func wellKnownLogicalClusterForFooWS() *corev1alpha1.LogicalCluster {
	return &corev1alpha1.LogicalCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       corev1alpha1.LogicalClusterName,
			Finalizers: []string{deletion.WorkspaceFinalizer},
			Annotations: map[string]string{
				tenancyv1alpha1.ExperimentalWorkspaceOwnerAnnotationKey: `{"username":"kcp-admin"}`,
				corev1alpha1.LogicalClusterTypeAnnotationKey:            "root:universal",
				core.LogicalClusterPathAnnotationKey:                    "root:foo",
			},
		},
		Spec: corev1alpha1.LogicalClusterSpec{
			Owner: &corev1alpha1.LogicalClusterOwner{
				APIVersion: tenancyv1beta1.SchemeGroupVersion.String(),
				Resource:   "workspaces",
				Name:       "foo",
				Cluster:    "root",
			},
			Initializers: []corev1alpha1.LogicalClusterInitializer{"root:organization", "system:apibindings"},
		},
	}
}

func wellKnownCRBForThisWS() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "workspace-admin",
			Annotations: map[string]string{},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				APIGroup: "rbac.authorization.k8s.io",
				Name:     "kcp-admin",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
}

func validateWellKnownCRBCreationAction(t *testing.T, actions []kcpclientgotesting.Action) {
	wasCRBCreated := false
	for _, action := range actions {
		if action.Matches("create", "clusterrolebindings") {
			createAction := action.(kcpclientgotesting.CreateAction)
			actualObj := createAction.GetObject().(*rbacv1.ClusterRoleBinding)
			expectedObj := wellKnownCRBForThisWS()
			if !equality.Semantic.DeepEqual(actualObj, expectedObj) {
				t.Errorf(cmp.Diff(actualObj, expectedObj))
			}
			wasCRBCreated = true
			break
		}
	}
	if !wasCRBCreated {
		t.Errorf("the ClusterRoleBinding wasn't created and validated")
	}
}

func validateWellKnownLogicalClusterActions(t *testing.T, actions []kcpclientgotesting.Action) {
	wasLogicalClusterCreated := false
	wasLogicalClusterUpdated := false
	expectedObj := wellKnownLogicalClusterForFooWS()
	for _, action := range actions {
		if action.Matches("create", "logicalclusters") {
			createAction := action.(kcpclientgotesting.CreateAction)
			actualObj := createAction.GetObject().(*corev1alpha1.LogicalCluster)

			if !equality.Semantic.DeepEqual(actualObj, expectedObj) {
				t.Errorf(cmp.Diff(actualObj, expectedObj))
			}
			wasLogicalClusterCreated = true
		}
		if action.Matches("update", "logicalclusters") {
			updateAction := action.(kcpclientgotesting.UpdateAction)
			expectedObjCopy := expectedObj.DeepCopy()
			expectedObjCopy.Status.Phase = "Initializing"
			actualObj := updateAction.GetObject().(*corev1alpha1.LogicalCluster)

			// this is a limitation of the fake client
			// to get an AlreadyExists error we need to assign
			// the shard annotation, which is still present on an update
			// in real world we wouldn't be seeing this annotation
			// since it is assigned by the kcp server
			delete(actualObj.Annotations, "kcp.dev/cluster")

			if !equality.Semantic.DeepEqual(actualObj, expectedObjCopy) {
				t.Errorf(cmp.Diff(actualObj, expectedObjCopy))
			}
			wasLogicalClusterUpdated = true
		}
	}
	if !wasLogicalClusterCreated {
		t.Errorf("LogicalCluster wasn't created and validated")
	}
	if !wasLogicalClusterUpdated {
		t.Errorf("LogicalCluster wasn't updated and validated")
	}
}

func shard(name string) *tenancyv1alpha1.ClusterWorkspaceShard {
	return &tenancyv1alpha1.ClusterWorkspaceShard{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: tenancyv1alpha1.ClusterWorkspaceShardSpec{
			BaseURL:     fmt.Sprintf("https://%s", name),
			ExternalURL: fmt.Sprintf("https://%s", name),
		},
	}
}

func workspaceType(name string) *tenancyv1alpha1.ClusterWorkspaceType {
	return &tenancyv1alpha1.ClusterWorkspaceType{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"kcp.dev/cluster": "root",
				"kcp.dev/path":    "root",
			},
		},
	}
}

func wellKnownClusterWorkspaceTypes() []*tenancyv1alpha1.ClusterWorkspaceType {
	type0 := workspaceType("root")
	type0.Spec.DefaultChildWorkspaceType = &tenancyv1alpha1.ClusterWorkspaceTypeReference{
		Name: "organization",
		Path: "root",
	}
	type1 := workspaceType("organization")
	type1.Spec.DefaultChildWorkspaceType = &tenancyv1alpha1.ClusterWorkspaceTypeReference{
		Name: "universal",
		Path: "root",
	}
	type1.Spec.Initializer = true
	type2 := workspaceType("universal")
	type2.Spec.DefaultChildWorkspaceType = &tenancyv1alpha1.ClusterWorkspaceTypeReference{
		Name: "universal",
		Path: "root",
	}
	type2.Spec.DefaultAPIBindings = []tenancyv1alpha1.APIExportReference{
		{
			Path:   "tenancy.kcp.dev",
			Export: "root",
		},
	}
	type2.Spec.Extend.With = []tenancyv1alpha1.ClusterWorkspaceTypeReference{
		{
			Name: "organization",
			Path: "root",
		},
	}
	return []*tenancyv1alpha1.ClusterWorkspaceType{type0, type1, type2}
}

func clearLastTransitionTimeOnWsConditions(ws *tenancyv1beta1.Workspace) {
	newConditions := make([]conditionsapi.Condition, 0, len(ws.Status.Conditions))
	for _, cond := range ws.Status.Conditions {
		cond.LastTransitionTime = metav1.Time{}
		newConditions = append(newConditions, cond)
	}
	ws.Status.Conditions = newConditions
}

func validateActionsVerbs(actualActions []kcpclientgotesting.Action, expectedActions []string) error {
	if len(actualActions) != len(expectedActions) {
		return fmt.Errorf("expected to get %d actions but got %d\nexpected=%v \n got=%v", len(expectedActions), len(actualActions), expectedActions, actionStrings(actualActions))
	}
	for i, a := range actualActions {
		if got, expected := actionString(a), expectedActions[i]; got != expected {
			return fmt.Errorf("at %d got %s, expected %s", i, got, expected)
		}
	}
	return nil
}

func actionString(a kcpclientgotesting.Action) string {
	if len(a.GetNamespace()) == 0 {
		return a.GetVerb() + ":" + a.GetResource().Resource
	}
	return a.GetVerb() + ":" + a.GetResource().Resource + ":" + a.GetNamespace()
}

func actionStrings(actions []kcpclientgotesting.Action) []string {
	res := make([]string, 0, len(actions))
	for _, a := range actions {
		res = append(res, actionString(a))
	}
	return res
}

func shardNameToBase36Sha224(name string) string {
	hash := sha256.Sum224([]byte(name))
	base36hash := strings.ToLower(base36.EncodeBytes(hash[:]))
	return base36hash[:8]
}
