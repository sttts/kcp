/*
Copyright 2021 The KCP Authors.
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

package builders

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	reststorage "k8s.io/apiserver/pkg/registry/rest"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	"github.com/kcp-dev/kcp/pkg/virtual/generic/apiserver"
	"github.com/kcp-dev/kcp/pkg/virtual/generic/builders"
	virtualworkspacesregistry "github.com/kcp-dev/kcp/pkg/virtual/workspaces/registry"
)

const WorkspacesVirtualWorkspaceName string = "workspaces"
const DefaultRootPathPrefix string = "/services/applications"

var scopeSets sets.String = sets.NewString("personal", "organization", "global")

type WorkspacesScopeKeyType string

const WorkspacesScopeKey WorkspacesScopeKeyType = "VirtualWorkspaceWorkspacesScope"

func WorkspacesVirtualWorkspaceBuilder(rootPathPrefix string) builders.VirtualWorkspaceBuilder {
	if !strings.HasSuffix(rootPathPrefix, "/") {
		rootPathPrefix += "/"
	}
	return builders.VirtualWorkspaceBuilder{
		Name: WorkspacesVirtualWorkspaceName,
		RootPathResolver: func(urlPath string, requestContext context.Context) (accepted bool, prefixToStrip string, completedContext context.Context) {
			completedContext = requestContext
			if path := urlPath; strings.HasPrefix(path, rootPathPrefix) {
				path = strings.TrimPrefix(path, rootPathPrefix)
				i := strings.Index(path, "/")
				if i == -1 {
					return
				}
				workspacesScope := path[:i]
				if !scopeSets.Has(workspacesScope) {
					return
				}

				return true, rootPathPrefix + workspacesScope, context.WithValue(requestContext, WorkspacesScopeKey, workspacesScope)
			}
			return
		},
		GroupsVersions: func(config apiserver.CompletedConfig) []builders.GroupVersionStorage {
			return []builders.GroupVersionStorage{
				{
					GroupVersion: tenancyv1alpha1.SchemeGroupVersion,
					ResourceStorage: map[string]reststorage.Storage{
						"workspaces": virtualworkspacesregistry.NewREST(
							config.GenericConfig.SharedInformerFactory,
							config.ExtraConfig.KcpClient,
							config.ExtraConfig.KcpInformer,
						),
					},
				},
			}
		},
	}
}
