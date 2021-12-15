package cluster

import (
	"context"
	"fmt"

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
	if v == nil {
		return "", fmt.Errorf("NO CLUSTER IN CONTEXT")
	}
	return v.(string), nil
}
