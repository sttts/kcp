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

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	"github.com/kcp-dev/kcp/pkg/client/informers/externalversions"
	"github.com/kcp-dev/kcp/pkg/cluster"
	"github.com/kcp-dev/kcp/pkg/controllerz"
	"github.com/kcp-dev/kcp/pkg/reconciler/deployment"
)

const numThreads = 2

var kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig")
var kubecontext = flag.String("context", "", "Context to use in the Kubeconfig file, instead of the current context")

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	flag.Parse()

	// TODO: export these and make them part of a KCP library that consumers can use
	clusterAwareKeyFunc := func(obj interface{}) (string, error) {
		acc, err := meta.Accessor(obj)
		if err != nil {
			return "", err
		}
		return acc.GetClusterName() + "|" + acc.GetNamespace() + "|" + acc.GetName(), nil
	}

	clusterAwareDecodeKeyFunc := func(key string) cache.QueueKey {
		parts := strings.Split(key, "|")
		clusterName := parts[0]
		namespace := parts[1]
		name := parts[2]
		return controllerz.NewKCPQueueKey(clusterName, namespace, name)
	}

	clusterNameIndex := func(obj interface{}) ([]string, error) {
		acc, err := meta.Accessor(obj)
		if err != nil {
			return []string{}, err
		}
		return []string{acc.GetClusterName()}, nil
	}

	clusterNameAndNamespaceIndex := func(obj interface{}) ([]string, error) {
		acc, err := meta.Accessor(obj)
		if err != nil {
			return []string{}, err
		}
		ns := acc.GetNamespace()
		if ns == "" {
			return []string{}, nil
		}
		return []string{acc.GetClusterName() + "|" + ns}, nil
	}

	clusterAwareNSKeyFunc := func(ctx context.Context, ns string) (string, error) {
		lcluster, err := cluster.FromContext(ctx)
		if err != nil {
			return "", err
		}
		return lcluster + "|" + ns, nil
	}

	clusterAwareNSNameKeyFunc := func(ctx context.Context, ns, name string) (string, error) {
		lcluster, err := cluster.FromContext(ctx)
		if err != nil {
			return "", err
		}
		return lcluster + "|" + ns + "|" + name, nil
	}

	listAllIndexName := "lcluster"
	namespaceIndexName := "lcluster+namespace"

	controllerConfig := cache.ControllerzConfig{
		ObjectKeyFunc:         clusterAwareKeyFunc,
		DecodeKeyFunc:         clusterAwareDecodeKeyFunc,
		ListAllIndex:          listAllIndexName,
		ListAllIndexFunc:      clusterNameIndex,
		ListAllIndexValueFunc: cluster.FromContext,
		NamespaceIndex:        namespaceIndexName,
		NamespaceIndexFunc:    clusterNameAndNamespaceIndex,
		NamespaceKeyFunc:      clusterAwareNSKeyFunc,
		NamespaceNameKeyFunc:  clusterAwareNSNameKeyFunc,
	}

	cache.Complete(controllerConfig)

	var overrides clientcmd.ConfigOverrides
	if *kubecontext != "" {
		overrides.CurrentContext = *kubecontext
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: *kubeconfig},
		&overrides,
	).ClientConfig()
	if err != nil {
		klog.Fatal(err)
	}

	crossKubeClient, err := kubernetes.NewClusterForConfig(cfg)
	if err != nil {
		panic(err)
	}

	crossKCPClient, err := kcpclient.NewClusterForConfig(cfg)
	if err != nil {
		panic(err)
	}
	// TODO: make a custom rest.HTTPClient that always does "*"
	kcpSharedInformerFactory := externalversions.NewSharedInformerFactoryWithOptions(crossKCPClient.Cluster("*"), 0)

	restClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		panic(err)
	}
	andyHTTPClient := &kcpHTTPClient{
		delegate: restClient,
	}
	andyKubeClient, err := kubernetes.NewForConfigAndClient(cfg, andyHTTPClient)

	// TODO: make a custom rest.HTTPClient that always does "*"
	kubeSharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(crossKubeClient.Cluster("*"), 0)

	deploymentController := deployment.NewController(
		kcpSharedInformerFactory.Cluster().V1alpha1().Clusters(),
		crossKubeClient,
		andyKubeClient.AppsV1(),
		kubeSharedInformerFactory.Apps().V1().Deployments(),
		kcpSharedInformerFactory.Cluster().V1alpha1().Clusters().Informer().HasSynced,
		kubeSharedInformerFactory.Apps().V1().Deployments().Informer().HasSynced,
	)

	kubeSharedInformerFactory.Start(ctx.Done())
	kcpSharedInformerFactory.Start(ctx.Done())

	deploymentController.Start(ctx, numThreads)
}

type kcpHTTPClient struct {
	delegate rest.HTTPClient
}

func (c *kcpHTTPClient) Do(req *http.Request) (*http.Response, error) {
	clusterName, err := cluster.FromContext(req.Context())
	if err != nil {
		// Couldn't find cluster name in context
		return c.delegate.Do(req)
	}

	if !strings.HasPrefix(req.URL.Path, "/clusters/") {
		originalPath := req.URL.Path

		// start with /clusters/$name
		req.URL.Path = "/clusters/" + clusterName

		// if the original path is relative, add a / separator
		if len(originalPath) > 0 && originalPath[0] != '/' {
			req.URL.Path += "/"
		}

		// finally append the original path
		req.URL.Path += originalPath
	}

	return c.delegate.Do(req)
}

func (c *kcpHTTPClient) Timeout() time.Duration {
	return c.delegate.Timeout()
}

func (c *kcpHTTPClient) Transport() http.RoundTripper {
	return c.delegate.Transport()
}
