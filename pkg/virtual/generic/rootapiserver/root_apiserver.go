package rootapiserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	// corev1 "k8s.io/api/core/v1"
	// kapierror "k8s.io/apimachinery/pkg/api/errors"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	// utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericapiserveroptions "k8s.io/apiserver/pkg/server/options"

	// genericmux "k8s.io/apiserver/pkg/server/mux"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/informers"
	kubeinformers "k8s.io/client-go/informers"
	rbacinformers "k8s.io/client-go/informers/rbac/v1"
	"k8s.io/client-go/kubernetes"

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

type RootPathResolverFunc func(urlPath string, context context.Context) (accepted bool, prefixToStrip string, completedContext context.Context)

type RootAPIExtraConfig struct {
	// we phrase it like this so we can build the post-start-hook, but no one can take more indirect dependencies on informers
	InformerStart func(stopCh <-chan struct{})

	KubeAPIServerClientConfig *rest.Config
	KubeInformers             kubeinformers.SharedInformerFactory

	GroupAPIServerBuilders []GroupAPIServerBuilder
	RootPathResolver RootPathResolverFunc

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
		var additionalConfig interface{}
		if groupAPIServerBuilder.AdditionalExtraConfigGetter != nil {
			groupAPIServerBuilder.AdditionalExtraConfigGetter(CompletedConfig{c})
		} 
		cfg := &virtualapiserver.GroupAPIServerConfig{
			GenericConfig: &genericapiserver.RecommendedConfig{Config: *c.GenericConfig.Config, SharedInformerFactory: c.GenericConfig.SharedInformerFactory},
			ExtraConfig: virtualapiserver.ExtraConfig{
				KubeAPIServerClientConfig: c.ExtraConfig.KubeAPIServerClientConfig,
				Codecs:                    legacyscheme.Codecs,
				Scheme:                    legacyscheme.Scheme,
				AdditionalConfig: 		   additionalConfig,
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
	c.GenericConfig.BuildHandlerChainFunc = c.getRootHandlerChain(delegateAPIServer)
	c.GenericConfig.RequestInfoResolver = c

	genericServer, err := c.GenericConfig.New("openshift-apiserver", delegateAPIServer)
	if err != nil {
		return nil, err
	}

	s := &RootAPIServer{
		GenericAPIServer: genericServer,
	}

	// register our poststarthooks
	s.GenericAPIServer.AddPostStartHookOrDie("just-a-placeholder-that-does-nothing", func(context genericapiserver.PostStartHookContext) error {
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

func (c completedConfig) getRootHandlerChain(delegateAPIServer genericapiserver.DelegationTarget) func(http.Handler, *genericapiserver.Config) http.Handler {
	return func(apiHandler http.Handler, genericConfig *genericapiserver.Config) http.Handler {
		return genericapiserver.DefaultBuildHandlerChain(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if accepted, prefixToStrip, context := c.ExtraConfig.RootPathResolver(req.URL.Path, req.Context()); accepted {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, prefixToStrip)
				req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, prefixToStrip)
				req = req.WithContext(genericapirequest.WithCluster(context, genericapirequest.Cluster{Name: "virtual"}))
				delegateAPIServer.UnprotectedHandler().ServeHTTP(w, req)
				return
			}
			http.NotFoundHandler().ServeHTTP(w, req)
		}), c.GenericConfig.Config)
	}
}


var _ genericapirequest.RequestInfoResolver = (*completedConfig)(nil)
func (c completedConfig) NewRequestInfo(req *http.Request) (*genericapirequest.RequestInfo, error) {
	defaultResolver := genericapiserver.NewRequestInfoResolver(c.GenericConfig.Config)	
	if accepted, prefixToStrip, _ := c.ExtraConfig.RootPathResolver(req.URL.Path, req.Context()); accepted {
		p := strings.TrimPrefix(req.URL.Path, prefixToStrip)
		rp := strings.TrimPrefix(req.URL.RawPath, prefixToStrip)
		r2 := new(http.Request)
		*r2 = *req
		r2.URL = new(url.URL)
		*r2.URL = *req.URL
		r2.URL.Path = p
		r2.URL.RawPath = rp
		return defaultResolver.NewRequestInfo(r2)
	}
	return defaultResolver.NewRequestInfo(req)
} 

type RootAPIServerBuilder struct {
	GroupAPIServerBuilders []GroupAPIServerBuilder
	InformerStarts []func(stopCh <-chan struct{})
	RootPathresolver RootPathResolverFunc	
}

func NewRootAPIConfig(kubeClientConfig *rest.Config, rootAPIServerBuilder RootAPIServerBuilder, secureServing *genericapiserveroptions.SecureServingOptionsWithLoopback, authenticationOptions *genericapiserveroptions.DelegatingAuthenticationOptions, authorizationOptions *genericapiserveroptions.DelegatingAuthorizationOptions) (*RootAPIConfig, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return nil, err
	}
	kubeInformers := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)

	genericConfig := genericapiserver.NewRecommendedConfig(legacyscheme.Codecs)
	// Current default values
	//Serializer:                   codecs,
	//ReadWritePort:                443,
	//BuildHandlerChainFunc:        DefaultBuildHandlerChain,
	//HandlerChainWaitGroup:        new(utilwaitgroup.SafeWaitGroup),
	//LegacyAPIGroupPrefixes:       sets.NewString(DefaultLegacyAPIPrefix),
	//DisabledPostStartHooks:       sets.NewString(),
	//HealthzChecks:                []healthz.HealthzChecker{healthz.PingHealthz, healthz.LogHealthz},
	//EnableIndex:                  true,
	//EnableDiscovery:              true,
	//EnableProfiling:              true,
	//EnableMetrics:                true,
	//MaxRequestsInFlight:          400,
	//MaxMutatingRequestsInFlight:  200,
	//RequestTimeout:               time.Duration(60) * time.Second,
	//MinRequestTimeout:            1800,
	//EnableAPIResponseCompression: utilfeature.DefaultFeatureGate.Enabled(features.APIResponseCompression),
	//LongRunningFunc: genericfilters.BasicLongRunningRequestCheck(sets.NewString("watch"), sets.NewString()),

	// TODO this is actually specific to the kubeapiserver
	//RuleResolver authorizer.RuleResolver
	genericConfig.SharedInformerFactory = kubeInformers
	genericConfig.ClientConfig = kubeClientConfig

	// these are set via options
	//SecureServing *SecureServingInfo
	//Authentication AuthenticationInfo
	//Authorization AuthorizationInfo
	//LoopbackClientConfig *restclient.Config
	// this is set after the options are overlayed to get the authorizer we need.
	//AdmissionControl      admission.Interface
	//ReadWritePort int
	//PublicAddress net.IP

	// these are defaulted sanely during complete
	//DiscoveryAddresses discovery.Addresses

	// TODO: genericConfig.ExternalAddress = ... allow a command line flag or it to be overriden by a top-level multiroot apiServer

	/*
	// previously overwritten.  I don't know why
	genericConfig.RequestTimeout = time.Duration(60) * time.Second
	genericConfig.MinRequestTimeout = int((time.Duration(60) * time.Minute).Seconds())
	genericConfig.MaxRequestsInFlight = -1 // TODO: allow configuring
	genericConfig.MaxMutatingRequestsInFlight = -1 // TODO configuring
	genericConfig.LongRunningFunc = apiserverconfig.IsLongRunningRequest
	*/

	if err := secureServing.ApplyTo(&genericConfig.Config.SecureServing, &genericConfig.Config.LoopbackClientConfig); err != nil {
		return nil, err
	}

	if err := authenticationOptions.ApplyTo(&genericConfig.Authentication, genericConfig.SecureServing, genericConfig.OpenAPIConfig); err != nil {
		return nil, err
	}
	if err := authorizationOptions.ApplyTo(&genericConfig.Authorization); err != nil {
		return nil, err
	}

	discoveryClient := cacheddiscovery.NewMemCacheClient(kubeClient.Discovery())
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)

	subjectLocator := NewSubjectLocator(kubeInformers.Rbac().V1())	
	ruleResolver := NewRuleResolver(kubeInformers.Rbac().V1())

	ret := &RootAPIConfig{
		GenericConfig: genericConfig,
		ExtraConfig: RootAPIExtraConfig{
			InformerStart:                      func(stopCh <-chan struct{}) {
				for _, informerStart := range rootAPIServerBuilder.InformerStarts {
					informerStart(stopCh)
				}
			},
			KubeAPIServerClientConfig:          kubeClientConfig,
			KubeInformers:                      kubeInformers, // TODO remove this and use the one from the genericconfig
			RuleResolver:                       ruleResolver,
			SubjectLocator:                     subjectLocator,
			RESTMapper:                         restMapper,
			GroupAPIServerBuilders: 			rootAPIServerBuilder.GroupAPIServerBuilders,
			RootPathResolver: rootAPIServerBuilder.RootPathresolver,
		},
	}

	return ret, ret.ExtraConfig.Validate()
}

func NewRuleResolver(informers rbacinformers.Interface) rbacregistryvalidation.AuthorizationRuleResolver {
	return rbacregistryvalidation.NewDefaultRuleResolver(
		&rbacauthorizer.RoleGetter{Lister: informers.Roles().Lister()},
		&rbacauthorizer.RoleBindingLister{Lister: informers.RoleBindings().Lister()},
		&rbacauthorizer.ClusterRoleGetter{Lister: informers.ClusterRoles().Lister()},
		&rbacauthorizer.ClusterRoleBindingLister{Lister: informers.ClusterRoleBindings().Lister()},
	)
}

func NewSubjectLocator(informers rbacinformers.Interface) rbacauthorizer.SubjectLocator {
	return rbacauthorizer.NewSubjectAccessEvaluator(
		&rbacauthorizer.RoleGetter{Lister: informers.Roles().Lister()},
		&rbacauthorizer.RoleBindingLister{Lister: informers.RoleBindings().Lister()},
		&rbacauthorizer.ClusterRoleGetter{Lister: informers.ClusterRoles().Lister()},
		&rbacauthorizer.ClusterRoleBindingLister{Lister: informers.ClusterRoleBindings().Lister()},
		"",
	)
}
