package cluster

import (
	"context"

	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"
)

type contextKey int

const clusterKey contextKey = iota

type clusterNamer interface {
	ClusterName() string
}

func NewContext(ctx context.Context, key cache.QueueKey) context.Context {
	if n, ok := key.(clusterNamer); ok {
		return context.WithValue(ctx, clusterKey, n.ClusterName())
	}
	return ctx
}

func FromContext(ctx context.Context) (string, error) {
	v := ctx.Value(clusterKey)
	if v != nil {
		return v.(string), nil
	}

	clusterName, err := request.ClusterNameFrom(ctx)
	if err != nil {
		return "", nil
	}

	return clusterName, nil
}
