/*
Copyright 2024 The KCP Authors.

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

package builder

import (
	"testing"

	kcpdynamic "github.com/kcp-dev/client-go/dynamic"
	kcpkubernetesclientset "github.com/kcp-dev/client-go/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/rest"

	"github.com/kcp-dev/kcp/pkg/virtual/framework"
	"github.com/kcp-dev/kcp/pkg/virtual/framework/dynamic/context"
	kcpinformers "github.com/kcp-dev/kcp/sdk/client/informers/externalversions"
)

func TestDigestUrl(t *testing.T) {
	rootPathPrefix := "/services/initializingworkspaces/"
	testCases := []struct {
		urlPath             string
		expectedAccept      bool
		expectedCluster     request.Cluster
		expectedKey         context.APIDomainKey
		expectedLogicalPath string
	}{
		{
			urlPath:             "/services/initializingworkspaces/test-initializer/clusters/test-cluster/apis/workload.kcp.io/v1alpha1/synctargets",
			expectedAccept:      true,
			expectedCluster:     request.Cluster{Name: "test-cluster", Wildcard: false, PartialMetadataRequest: false},
			expectedKey:         "test-initializer",
			expectedLogicalPath: "/services/initializingworkspaces/test-initializer/clusters/test-cluster",
		},
		{
			urlPath:             "/services/initializingworkspaces/test-initializer/clusters/*/apis/workload.kcp.io/v1alpha1/synctargets",
			expectedAccept:      true,
			expectedCluster:     request.Cluster{Name: "", Wildcard: true, PartialMetadataRequest: false},
			expectedKey:         "test-initializer",
			expectedLogicalPath: "/services/initializingworkspaces/test-initializer/clusters/*",
		},
		{
			urlPath:             "/services/initializingworkspaces/test-initializer/clusters/",
			expectedAccept:      true,
			expectedCluster:     request.Cluster{Name: "", Wildcard: false, PartialMetadataRequest: false},
			expectedKey:         "test-initializer",
			expectedLogicalPath: "/services/initializingworkspaces/test-initializer/clusters",
		},
		{
			urlPath:             "/services/initializingworkspaces/test-initializer/clusters/*",
			expectedAccept:      true,
			expectedCluster:     request.Cluster{Name: "", Wildcard: true, PartialMetadataRequest: false},
			expectedKey:         "test-initializer",
			expectedLogicalPath: "/services/initializingworkspaces/test-initializer/clusters/*",
		},
		{
			urlPath:             "/services/initializingworkspaces/test-initializer/",
			expectedAccept:      false,
			expectedCluster:     request.Cluster{Name: "", Wildcard: false, PartialMetadataRequest: false},
			expectedKey:         "",
			expectedLogicalPath: "",
		},
		{
			urlPath:             "/services/initializingworkspaces/",
			expectedAccept:      false,
			expectedCluster:     request.Cluster{Name: "", Wildcard: false, PartialMetadataRequest: false},
			expectedKey:         "",
			expectedLogicalPath: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.urlPath, func(t *testing.T) {
			cluster, key, logicalPath, accepted := digestUrl(tc.urlPath, rootPathPrefix)
			require.Equal(t, tc.expectedAccept, accepted, "Accepted should match expected value")
			require.Equal(t, tc.expectedCluster, cluster, "Cluster should match expected value")
			require.Equal(t, tc.expectedKey, key, "Key should match expected value")
			require.Equal(t, tc.expectedLogicalPath, logicalPath, "LogicalPath should match expected value")
		})
	}
}

func TestIsLogicalClusterRequest(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/apis/core.kcp.io/v1alpha1/logicalclusters", true},
		{"/apis/othergroup/v1alpha1/logicalclusters", false},
		{"/apis/core.kcp.io/v1alpha1/otherresource", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := isLogicalClusterRequest(tc.path)
			require.Equal(t, tc.expected, result, "Result should match expected value")
		})
	}
}

func TestBuildVirtualWorkspace(t *testing.T) {
	rootPathPrefix := "/services/initializingworkspaces/"
	cfg := &rest.Config{
		Host: "https://example.com",
	}
	kubeClusterClient, err := kcpkubernetesclientset.NewForConfig(cfg)
	require.NoError(t, err, "ClusterClientSet should not return an error")
	dynamicClusterClient, err := kcpdynamic.NewForConfig(cfg)
	require.NoError(t, err, "ClusterClientSet should not return an error")
	wildcardKcpInformers := kcpinformers.NewSharedInformerFactory(nil, 0)

	virtualWorkspaces, err := BuildVirtualWorkspace(cfg, rootPathPrefix, dynamicClusterClient, kubeClusterClient, wildcardKcpInformers)
	require.NoError(t, err, "BuildVirtualWorkspace should not return an error")

	assert.Len(t, virtualWorkspaces, 3, "There should be three virtual workspaces")
	expectedNames := map[string]struct{}{
		wildcardLogicalClustersName: {},
		logicalClustersName:         {},
		workspaceContentName:        {},
	}

	for _, vw := range virtualWorkspaces {
		t.Run(vw.Name, func(t *testing.T) {
			assert.NotNil(t, vw.VirtualWorkspace, "VirtualWorkspace should not be nil")
			assert.Implements(t, (*framework.RootPathResolver)(nil), vw.VirtualWorkspace, "VirtualWorkspace should implement RootPathResolver")
			assert.Implements(t, (*authorizer.Authorizer)(nil), vw.VirtualWorkspace, "VirtualWorkspace should implement Authorizer")
			assert.Implements(t, (*framework.ReadyChecker)(nil), vw.VirtualWorkspace, "VirtualWorkspace should implement ReadyChecker")
			assert.NotNil(t, vw.VirtualWorkspace.Register, "Register should not be nil")

			_, exists := expectedNames[vw.Name]
			assert.True(t, exists, "VirtualWorkspace name should be one of the expected names")
		})
	}
}
