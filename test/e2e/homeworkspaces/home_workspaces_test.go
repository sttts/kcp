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

package homeworkspaces

import (
	"context"
	"path"
	"testing"

	kcpkubernetesclientset "github.com/kcp-dev/client-go/kubernetes"
	"github.com/kcp-dev/logicalcluster/v3"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	virtualoptions "github.com/kcp-dev/kcp/cmd/virtual-workspaces/options"
	"github.com/kcp-dev/kcp/pkg/apis/core"
	corev1alpha1 "github.com/kcp-dev/kcp/pkg/apis/core/v1alpha1"
	kcpclientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	kcpclusterclientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/cluster"
	"github.com/kcp-dev/kcp/test/e2e/framework"
)

func TestUserHomeWorkspaces(t *testing.T) {
	t.Parallel()
	framework.Suite(t, "control-plane")

	type clientInfo struct {
		Token string
	}

	type runningServer struct {
		framework.RunningServer
		kubeClusterClient             kcpkubernetesclientset.ClusterInterface
		rootShardKcpClusterClient     kcpclusterclientset.ClusterInterface
		kcpUserClusterClients         []kcpclusterclientset.ClusterInterface
		virtualPersonalClusterClients []VirtualClusterClient
	}

	var testCases = []struct {
		name    string
		kcpArgs []string
		work    func(ctx context.Context, t *testing.T, server runningServer)
	}{
		{
			name: "Create a workspace in the non-existing home and have it created automatically through ~",
			kcpArgs: []string{
				"--home-workspaces-home-creator-groups",
				"team-1",
			},
			work: func(ctx context.Context, t *testing.T, server runningServer) {
				t.Helper()

				kcpUser1Client := server.kcpUserClusterClients[0]
				kcpUser2Client := server.kcpUserClusterClients[1]

				t.Logf("Get ~ Home workspace URL for user-1")
				createdHome, err := kcpUser1Client.Cluster(core.RootCluster.Path()).TenancyV1alpha1().Workspaces().Get(ctx, "~", metav1.GetOptions{})
				require.NoError(t, err, "user-1 should be able to get ~ workspace")
				require.NotEqual(t, metav1.Time{}, createdHome.CreationTimestamp, "should have a creation timestamp, i.e. is not virtual")
				require.Equal(t, corev1alpha1.LogicalClusterPhaseReady, createdHome.Status.Phase, "created home workspace should be ready")

				t.Logf("Get the logical cluster inside user:user-1 (alias of ~)")
				_, err = kcpUser1Client.Cluster(logicalcluster.NewPath("user:user-1")).CoreV1alpha1().LogicalClusters().Get(ctx, "cluster", metav1.GetOptions{})
				require.NoError(t, err, "user-1 should be able to get a logical cluster in home workspace")

				t.Logf("Get ~ Home workspace URL for user-2")
				_, err = kcpUser2Client.Cluster(core.RootCluster.Path()).TenancyV1alpha1().Workspaces().Get(ctx, "~", metav1.GetOptions{})
				require.EqualError(t, err, `workspaces.tenancy.kcp.io "~" is forbidden: User "user-2" cannot create resource "workspaces" in API group "tenancy.kcp.io" at the cluster scope: workspace access not permitted`, "user-2 should not be allowed to get his home workspace even before it exists")
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			tokenAuthFile := framework.WriteTokenAuthFile(t)
			server := framework.PrivateKcpServer(t, framework.WithCustomArguments(append(framework.TestServerArgsWithTokenAuthFile(tokenAuthFile), testCase.kcpArgs...)...))
			ctx, cancelFunc := context.WithCancel(context.Background())
			t.Cleanup(cancelFunc)

			// create non-virtual clients
			kcpConfig := server.BaseConfig(t)
			rootShardCfg := server.RootShardSystemMasterBaseConfig(t)
			kubeClusterClient, err := kcpkubernetesclientset.NewForConfig(kcpConfig)
			require.NoError(t, err, "failed to construct client for server")
			rootShardKcpClusterClient, err := kcpclusterclientset.NewForConfig(rootShardCfg)
			require.NoError(t, err, "failed to construct client for server")

			// create kcp client and virtual clients for all users requested
			var kcpUserClusterClients []kcpclusterclientset.ClusterInterface
			var virtualPersonalClusterClients []VirtualClusterClient
			for _, ci := range []clientInfo{{Token: "user-1-token"}, {Token: "user-2-token"}} {
				userConfig := framework.ConfigWithToken(ci.Token, rest.CopyConfig(kcpConfig))
				virtualPersonalClusterClients = append(virtualPersonalClusterClients, &virtualClusterClient{config: userConfig})
				kcpUserClusterClient, err := kcpclusterclientset.NewForConfig(userConfig)
				require.NoError(t, err)
				kcpUserClusterClients = append(kcpUserClusterClients, kcpUserClusterClient)
			}

			testCase.work(ctx, t, runningServer{
				RunningServer:                 server,
				kubeClusterClient:             kubeClusterClient,
				rootShardKcpClusterClient:     rootShardKcpClusterClient,
				kcpUserClusterClients:         kcpUserClusterClients,
				virtualPersonalClusterClients: virtualPersonalClusterClients,
			})
		})
	}
}

type VirtualClusterClient interface {
	Cluster(cluster logicalcluster.Path) kcpclientset.Interface
}

type virtualClusterClient struct {
	config *rest.Config
}

func (c *virtualClusterClient) Cluster(cluster logicalcluster.Path) kcpclientset.Interface {
	config := rest.CopyConfig(c.config)
	config.Host += path.Join(virtualoptions.DefaultRootPathPrefix, "workspaces", cluster.String())
	return kcpclientset.NewForConfigOrDie(config)
}
