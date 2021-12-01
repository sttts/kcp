package cmd

import (
	"k8s.io/klog/v2"

	genericapiserver "k8s.io/apiserver/pkg/server"
	genericapiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"

	virtualrootapiserver "github.com/kcp-dev/kcp/pkg/virtual/generic/rootapiserver"
)

func RunRootAPIServer(kubeClientConfig *rest.Config, secureServing *genericapiserveroptions.SecureServingOptionsWithLoopback, authenticationOptions *genericapiserveroptions.DelegatingAuthenticationOptions, authorizationOptions *genericapiserveroptions.DelegatingAuthorizationOptions, stopCh <-chan struct{}, informerStarts virtualrootapiserver.InformerStartsFunc, rootAPIServerBuilders ...virtualrootapiserver.RootAPIServerBuilder) error {
	rootAPIServerConfig, err := virtualrootapiserver.NewRootAPIConfig(kubeClientConfig, secureServing, authenticationOptions, authorizationOptions, informerStarts, rootAPIServerBuilders...)
	if err != nil {
		return err
	}

	completedRootAPIServerConfig := rootAPIServerConfig.Complete()
	rootAPIServer, err := completedRootAPIServerConfig.New(genericapiserver.NewEmptyDelegate())
	if err != nil {
		return err
	}
	preparedRootAPIServer := rootAPIServer.GenericAPIServer.PrepareRun()

	// this **must** be done after PrepareRun() as it sets up the openapi endpoints
	if err := completedRootAPIServerConfig.WithOpenAPIAggregationController(preparedRootAPIServer.GenericAPIServer); err != nil {
		return err
	}

	klog.Infof("Starting master on %s (%s)", rootAPIServerConfig.GenericConfig.ExternalAddress, version.Get().String())

	return preparedRootAPIServer.Run(stopCh)
}
