package controllerz

import (
	"context"
	"fmt"
	"strings"

	"github.com/kcp-dev/kcp/pkg/cluster"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
)

type KCPQueueKey struct {
	clusterName, namespace, name string
}

func NewKCPQueueKey(clusterName, namespace, name string) *KCPQueueKey {
	return &KCPQueueKey{
		clusterName: clusterName,
		namespace:   namespace,
		name:        name,
	}
}

func (k *KCPQueueKey) ClusterName() string {
	return k.clusterName
}

func (k *KCPQueueKey) Namespace() string {
	return k.namespace
}

func (k *KCPQueueKey) Name() string {
	return k.name
}

func clusterAwareKeyFunc(obj interface{}) (string, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}
	if acc.GetNamespace() != "" {
		return NamespaceScopedKey(acc.GetClusterName(), acc.GetNamespace(), acc.GetName()), nil
	}
	return ClusterScopedKey(acc.GetClusterName(), acc.GetName()), nil
}

func clusterAwareDecodeKeyFunc(key string) (cache.QueueKey, error) {
	clusterAndKey := strings.Split(key, "$")
	if len(clusterAndKey) != 2 {
		return nil, fmt.Errorf("unexpected key format %q", key)
	}

	clusterName := clusterAndKey[0]

	keyParts := strings.Split(clusterAndKey[1], "/")
	switch len(keyParts) {
	case 1:
		return NewKCPQueueKey(clusterName, "", keyParts[0]), nil
	case 2:
		return NewKCPQueueKey(clusterName, keyParts[0], keyParts[1]), nil
	}

	return nil, fmt.Errorf("unexpected key format %q", key)
}

func clusterNameIndex(obj interface{}) ([]string, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return []string{}, err
	}
	return []string{acc.GetClusterName()}, nil
}

func clusterNameAndNamespaceIndex(obj interface{}) ([]string, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return []string{}, err
	}
	ns := acc.GetNamespace()
	if ns == "" {
		return []string{}, nil
	}
	return []string{ClusterScopedKey(acc.GetClusterName(), ns)}, nil
}

func clusterAwareNSNameKeyFunc(ctx context.Context, ns, name string) (string, error) {
	lcluster, err := cluster.FromContext(ctx)
	if err != nil {
		return "", nil
	}
	return NamespaceScopedKey(lcluster, ns, name), nil
}

func clusterAwareNameKeyFunc(ctx context.Context, name string) (string, error) {
	lcluster, err := cluster.FromContext(ctx)
	if err != nil {
		return "", nil
	}
	return ClusterScopedKey(lcluster, name), nil
}

func ClusterScopedKey(cluster, name string) string {
	return cluster + "$" + name
}

func NamespaceScopedKey(cluster, namespace, name string) string {
	return cluster + "$" + namespace + "/" + name
}

func EnableLogicalClusters() {
	controllerConfig := cache.ControllerzConfig{
		ObjectKeyFunc:         clusterAwareKeyFunc,
		DecodeKeyFunc:         clusterAwareDecodeKeyFunc,
		ListAllIndexFunc:      clusterNameIndex,
		ListAllIndexValueFunc: cluster.FromContext,
		NamespaceIndexFunc:    clusterNameAndNamespaceIndex,
		NamespaceKeyFunc:      clusterAwareNameKeyFunc,
		NamespaceNameKeyFunc:  clusterAwareNSNameKeyFunc,
		NameKeyFunc:           clusterAwareNameKeyFunc,
		NewSyncContextFunc:    cluster.NewContext,
	}

	cache.Complete(controllerConfig)
}
