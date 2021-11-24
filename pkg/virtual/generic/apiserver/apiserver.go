package apiserver

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	restclient "k8s.io/client-go/rest"
)

type RestStorageBuidler func(config CompletedConfig) (rest.Storage, error)

type ExtraConfig struct {
	KubeAPIServerClientConfig *restclient.Config
	AdditionalConfig          interface{}

	GroupVersion schema.GroupVersion
	StorageBuilders map[string] RestStorageBuidler

	// TODO these should all become local eventually
	Scheme *runtime.Scheme
	Codecs serializer.CodecFactory

	makeStorage sync.Once
	storage     map[string]rest.Storage
	storageErr  error
}

type GroupAPIServerConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// GroupAPIServer contains state for a Kubernetes cluster master/api server.
type GroupAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *GroupAPIServerConfig) Complete() completedConfig {
	cfg := completedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}

	return cfg
}

// New returns a new instance of VirtualWorkspaceAPIServer from the given config.
func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*GroupAPIServer, error) {
	genericServer, err := c.GenericConfig.New(c.ExtraConfig.GroupVersion.Group + "-virtual-workspace-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	s := &GroupAPIServer{
		GenericAPIServer: genericServer,
	}

	storage, err := c.RESTStorage()
	if err != nil {
		return nil, err
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(c.ExtraConfig.GroupVersion.Group, c.ExtraConfig.Scheme, metav1.ParameterCodec, c.ExtraConfig.Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[c.ExtraConfig.GroupVersion.Version] = storage
	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return s, nil
}

func (c *completedConfig) RESTStorage() (map[string]rest.Storage, error) {
	c.ExtraConfig.makeStorage.Do(func() {
		c.ExtraConfig.storage, c.ExtraConfig.storageErr = c.newRESTStorage()
	})

	return c.ExtraConfig.storage, c.ExtraConfig.storageErr
}

func (c *completedConfig) newRESTStorage() (map[string]rest.Storage, error) {
	storage := map[string]rest.Storage{}
	for resource, storageBuilder := range c.ExtraConfig.StorageBuilders {
		restStorage, err := storageBuilder(CompletedConfig{completedConfig: c})
		if err != nil {
			return nil, err
		}
		storage[resource] = restStorage
	}

	return storage, nil
}
