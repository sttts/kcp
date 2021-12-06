package builders

import (
	"context"
	"strings"

	builders "github.com/kcp-dev/kcp/pkg/virtual/generic/builders"
	virtualworkspacesregistry "github.com/kcp-dev/kcp/pkg/virtual/workspaces/registry"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/registry/rest"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
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
		RootPathresolver: func(urlPath string, requestContext context.Context) (accepted bool, prefixToStrip string, completedContext context.Context) {
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
		GroupAPIServerBuilders: []builders.APIGroupAPIServerBuilder{
			{
				GroupVersion: tenancyv1alpha1.SchemeGroupVersion,
				AdditionalExtraConfigGetter: func(mainConfig builders.MainConfigProvider) interface{} {
					return nil
				},
				StorageBuilders: map[string]builders.RestStorageBuidler{
					"workspaces": func(config builders.APIGroupConfigProvider) (rest.Storage, error) {
						kubeInformers := config.CompletedConfig().SharedInformerFactory
						kcpInformer := config.SharedExtraConfig().KcpInformer
						kcpClient := config.SharedExtraConfig().KcpClient
						return virtualworkspacesregistry.NewREST(kubeInformers, kcpClient, kcpInformer), nil
					},
				},
			},
		},
	}
}
