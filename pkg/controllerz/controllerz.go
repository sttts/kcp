package controllerz

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
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

func decodeKey(key string) (cluster, namespace, name string, err error) {
	clusterAndKey := strings.Split(key, "$")
	if len(clusterAndKey) != 2 {
		err = fmt.Errorf("unexpected key format %q", key)
		return
	}

	cluster = clusterAndKey[0]

	keyParts := strings.Split(clusterAndKey[1], "/")
	switch len(keyParts) {
	case 1:
		namespace = keyParts[0]
		return
	case 2:
		namespace = keyParts[0]
		name = keyParts[1]
		return
	}

	err = fmt.Errorf("unexpected key format %q", key)
	return
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
	scope := rest.ScopeFrom(ctx)
	if scope == nil {
		return ns + "/" + name, nil
	}
	return NamespaceScopedKey(scope.Name(), ns, name), nil
}

func clusterAwareNameKeyFunc(ctx context.Context, name string) (string, error) {
	scope := rest.ScopeFrom(ctx)
	if scope == nil {
		return name, nil
	}
	return ClusterScopedKey(scope.Name(), name), nil
}

func ClusterScopedKey(cluster, name string) string {
	// If name is already qualified with a cluster, strip off the cluster portion.
	// TODO(ncdc): see if we want to restructure how/when this is called so we don't need to do this.
	if i := strings.Index(name, "$"); i != -1 {
		name = name[i+1:]

	}
	return cluster + "$" + name
}

func NamespaceScopedKey(cluster, namespace, name string) string {
	return cluster + "$" + namespace + "/" + name
}

type Scope interface {
	rest.Scope
	Wildcard() bool
}

type scope struct {
	name     string
	wildcard bool
}

type ScopeOptionFunc func(s *scope)

func WildcardScope(w bool) ScopeOptionFunc {
	return func(s *scope) {
		s.wildcard = w
	}
}

func NewScope(name string, options ...ScopeOptionFunc) *scope {
	s := &scope{
		name: name,
	}

	for _, o := range options {
		o(s)
	}

	return s
}

func (s *scope) Name() string {
	return s.name
}

func (s *scope) Wildcard() bool {
	return s.wildcard
}

func (s *scope) CacheKey(in string) string {
	if strings.Contains(in, "$") {
		// already scoped
		return in
	}

	if s.wildcard {
		return in
	}

	return ClusterScopedKey(s.name, in)
}

func (s *scope) ScopeRequest(req *http.Request) error {
	if strings.HasPrefix(req.URL.Path, "/clusters/") {
		// TODO(ncdc): don't panic
		panic(fmt.Errorf("scope=%q; req already had a cluster: %q", s.name, req.URL.Path))
	} else {
		originalPath := req.URL.Path

		// start with /clusters/$name
		req.URL.Path = "/clusters/" + s.name

		// if the original path is relative, add a / separator
		if len(originalPath) > 0 && originalPath[0] != '/' {
			req.URL.Path += "/"
		}

		// finally append the original path
		req.URL.Path += originalPath
	}
	return nil
}

func ScopeFromContext(ctx context.Context) (Scope, error) {
	s := rest.ScopeFrom(ctx)
	if s == nil {
		return nil, nil
	}
	if kcpScope, ok := s.(Scope); ok {
		return kcpScope, nil
	}
	return nil, fmt.Errorf("unable to find a KCP scope in ctx")
}

type scoper struct{}

// func (s *scoper) ScopeFromContext(ctx context.Context) (rest.Scope, error) {
// 	return nil, nil
// }

func (s *scoper) ScopeFromObject(obj metav1.Object) (rest.Scope, error) {
	return NewScope(obj.GetClusterName()), nil
}

func (s *scoper) ScopeFromKey(key string) (rest.Scope, error) {
	cluster, _, _, err := decodeKey(key)
	if err != nil {
		return nil, err
	}

	return &scope{name: cluster}, nil
}

func EnableLogicalClusters() {
	controllerConfig := cache.ControllerzConfig{
		ObjectKeyFunc:    clusterAwareKeyFunc,
		DecodeKeyFunc:    clusterAwareDecodeKeyFunc,
		ListAllIndexFunc: clusterNameIndex,
		// ListAllIndexValueFunc: cluster.FromContext,
		NamespaceIndexFunc:   clusterNameAndNamespaceIndex,
		NamespaceKeyFunc:     clusterAwareNameKeyFunc,
		NamespaceNameKeyFunc: clusterAwareNSNameKeyFunc,
		NameKeyFunc:          clusterAwareNameKeyFunc,
		// NewSyncContextFunc:    cluster.NewContext,
		Scoper: &scoper{},
	}

	cache.Complete(controllerConfig)
}
