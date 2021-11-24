package rootapiserver

import (
	"context"
	"fmt"
	// "net/http"
	"time"

	// corev1 "k8s.io/api/core/v1"
	// kapierror "k8s.io/apimachinery/pkg/api/errors"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	// utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapiserver "k8s.io/apiserver/pkg/server"

	// genericmux "k8s.io/apiserver/pkg/server/mux"
	kubeinformers "k8s.io/client-go/informers"
	// corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	// rbacrest "k8s.io/kubernetes/pkg/registry/rbac/rest"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"

	virtualapiserver "github.com/kcp-dev/kcp/pkg/virtual/generic/apiserver"
)

type RootAPIExtraConfig struct {
	// we phrase it like this so we can build the post-start-hook, but no one can take more indirect dependencies on informers
	InformerStart func(stopCh <-chan struct{})

	KubeAPIServerClientConfig *rest.Config
	KubeInformers             kubeinformers.SharedInformerFactory

	GroupAPIServerBuilders []GroupAPIServerBuilder
	RootPathResolver func(urlPath string, context context.Context) (bool, context.Context)

	// these are all required to build our storage
	RuleResolver   rbacregistryvalidation.AuthorizationRuleResolver
	SubjectLocator rbacauthorizer.SubjectLocator

	RESTMapper                *restmapper.DeferredDiscoveryRESTMapper
}

// Validate helps ensure that we build this config correctly, because there are lots of bits to remember for now
func (c *RootAPIExtraConfig) Validate() error {
	ret := []error{}

	if c.KubeInformers == nil {
		ret = append(ret, fmt.Errorf("KubeInformers is required"))
	}
	if c.RuleResolver == nil {
		ret = append(ret, fmt.Errorf("RuleResolver is required"))
	}
	if c.SubjectLocator == nil {
		ret = append(ret, fmt.Errorf("SubjectLocator is required"))
	}
	if c.RESTMapper == nil {
		ret = append(ret, fmt.Errorf("RESTMapper is required"))
	}

	return utilerrors.NewAggregate(ret)
}

type RootAPIConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   RootAPIExtraConfig
}

// RootAPIServer is only responsible for serving the APIs for the virtual workspace
// at a given root path or root path family
// It does NOT expose oauth, related oauth endpoints, or any kube APIs.
type RootAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *RootAPIExtraConfig
}

type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *RootAPIConfig) Complete() completedConfig {
	cfg := completedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}

	return cfg
}

type GroupAPIServerBuilder struct {
	GroupVersion schema.GroupVersion
	AdditionalExtraConfigGetter func(rootAPIServerCompletedConfig CompletedConfig) interface{}
	StorageBuilders map[string]virtualapiserver.RestStorageBuidler
}

func (c *completedConfig) withGroupAPIServer(groupAPIServerBuilder GroupAPIServerBuilder) apiServerAppenderFunc {
	return func(delegateAPIServer genericapiserver.DelegationTarget) (genericapiserver.DelegationTarget, error) {
		cfg := &virtualapiserver.GroupAPIServerConfig{
			GenericConfig: &genericapiserver.RecommendedConfig{Config: *c.GenericConfig.Config, SharedInformerFactory: c.GenericConfig.SharedInformerFactory},
			ExtraConfig: virtualapiserver.ExtraConfig{
				KubeAPIServerClientConfig: c.ExtraConfig.KubeAPIServerClientConfig,
				Codecs:                    legacyscheme.Codecs,
				Scheme:                    legacyscheme.Scheme,
				AdditionalConfig: 		   groupAPIServerBuilder.AdditionalExtraConfigGetter(CompletedConfig{c}),
				GroupVersion: groupAPIServerBuilder.GroupVersion,
				StorageBuilders: groupAPIServerBuilder.StorageBuilders,
			},
		}
		config := cfg.Complete()
		server, err := config.New(delegateAPIServer)
		if err != nil {
			return nil, err
		}
	
		return server.GenericAPIServer, nil
	}
}

func (c *completedConfig) WithOpenAPIAggregationController(delegatedAPIServer *genericapiserver.GenericAPIServer) error {
	return nil
}

type apiServerAppenderFunc func(delegateAPIServer genericapiserver.DelegationTarget) (genericapiserver.DelegationTarget, error)

func addAPIServerOrDie(delegateAPIServer genericapiserver.DelegationTarget, apiServerAppenderFn apiServerAppenderFunc) genericapiserver.DelegationTarget {
	delegateAPIServer, err := apiServerAppenderFn(delegateAPIServer)
	if err != nil {
		klog.Fatal(err)
	}

	return delegateAPIServer
}

func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*RootAPIServer, error) {
	delegateAPIServer := delegationTarget

	for _, groupAPIServerBuilder := range c.ExtraConfig.GroupAPIServerBuilders {
		delegateAPIServer = addAPIServerOrDie(delegateAPIServer, c.withGroupAPIServer(groupAPIServerBuilder))
	}

	// TODO: Positionner c.GenericConfig.BuildHandlerChainFunc pour appliquer le RootPathResolver
	// Attention: vérifier que les delegate ne sont pas créés avec cette BuildHandlerChain spécifique.
	// => Peut-être spécialement pour le withOpenAPI... Mais c'est peut-être pas un problème
	// si on crée pas un autre apiServer + Config.  
	genericServer, err := c.GenericConfig.New("openshift-apiserver", delegateAPIServer)
	if err != nil {
		return nil, err
	}

	s := &RootAPIServer{
		GenericAPIServer: genericServer,
	}

	// register our poststarthooks
	s.GenericAPIServer.AddPostStartHookOrDie("just-a-placeholder-that-does-nothing",
		func(context genericapiserver.PostStartHookContext) error {
			return nil
		})
	s.GenericAPIServer.AddPostStartHookOrDie("virtual-workspace-startinformers", func(context genericapiserver.PostStartHookContext) error {
		c.ExtraConfig.InformerStart(context.StopCh)
		return nil
	})
	s.GenericAPIServer.AddPostStartHookOrDie("openshift.io-restmapperupdater", func(context genericapiserver.PostStartHookContext) error {
		go func() {
			wait.Until(func() {
				c.ExtraConfig.RESTMapper.Reset()
			}, 10*time.Second, context.StopCh)
		}()
		return nil

	})

	return s, nil
}
