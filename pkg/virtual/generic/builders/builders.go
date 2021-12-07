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

	"k8s.io/apimachinery/pkg/runtime/schema"
	restStorage "k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"

	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	kcpinformer "github.com/kcp-dev/kcp/pkg/client/informers/externalversions"
)

type MainConfigProvider interface {
	CompletedConfig() genericapiserver.CompletedConfig
	SharedExtraConfig() SharedExtraConfig
}

type APIGroupConfigProvider interface {
	CompletedGenericConfig() genericapiserver.CompletedConfig
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
