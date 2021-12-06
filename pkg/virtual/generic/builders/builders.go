package builders

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	restStorage "k8s.io/apiserver/pkg/registry/rest"

	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"

	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	kcpinformer "github.com/kcp-dev/kcp/pkg/client/informers/externalversions"

	genericapiserver "k8s.io/apiserver/pkg/server"
)

type MainConfigProvider interface {
	CompletedConfig() genericapiserver.CompletedConfig
	SharedExtraConfig() SharedExtraConfig
}

type APIGroupConfigProvider interface {
	CompletedConfig() genericapiserver.CompletedConfig
	SharedExtraConfig() SharedExtraConfig
	AdditionalConfig() interface{}
}

type SharedExtraConfig struct {
	KubeAPIServerClientConfig *rest.Config
	KcpClient                 *kcpclient.Clientset
	KcpInformer               kcpinformer.SharedInformerFactory
	RuleResolver              rbacregistryvalidation.AuthorizationRuleResolver
	SubjectLocator            rbacauthorizer.SubjectLocator
	RESTMapper                *restmapper.DeferredDiscoveryRESTMapper
}

type RestStorageBuidler func(apiGroupConfig APIGroupConfigProvider) (restStorage.Storage, error)

type APIGroupAPIServerBuilder struct {
	GroupVersion                schema.GroupVersion
	AdditionalExtraConfigGetter func(mainConfig MainConfigProvider) interface{}
	StorageBuilders             map[string]RestStorageBuidler
}

type RootPathResolverFunc func(urlPath string, context context.Context) (accepted bool, prefixToStrip string, completedContext context.Context)

type VirtualWorkspaceBuilder struct {
	Name                   string
	GroupAPIServerBuilders []APIGroupAPIServerBuilder
	RootPathresolver       RootPathResolverFunc
}

type VirtualWorkspaceBuilderProvider interface {
	VirtualWorkspaceBuilder() VirtualWorkspaceBuilder
}

var _ VirtualWorkspaceBuilderProvider = VirtualWorkspaceBuilderProviderFunc(nil)

type VirtualWorkspaceBuilderProviderFunc func() VirtualWorkspaceBuilder

func (f VirtualWorkspaceBuilderProviderFunc) VirtualWorkspaceBuilder() VirtualWorkspaceBuilder {
	return f()
}
