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

package deployment

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	clusterinformers "github.com/kcp-dev/kcp/pkg/client/informers/externalversions/cluster/v1alpha1"
	clusterlisters "github.com/kcp-dev/kcp/pkg/client/listers/cluster/v1alpha1"
)

const resyncPeriod = 10 * time.Hour
const controllerName = "deployment"

// NewController returns a new Controller which splits new Deployment objects
// into N virtual Deployments labeled for each Cluster that exists at the time
// the Deployment is created.
func NewController(
	clusterInformer clusterinformers.ClusterInformer,
	kubeClient kubernetes.ClusterInterface,
	deploymentClient appsv1client.DeploymentsGetter,
	deploymentInformer appsinformers.DeploymentInformer,
	syncFuncs ...cache.InformerSynced,
) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	c := &Controller{
		queue:            queue,
		clusterLister:    clusterInformer.Lister(),
		kubeClient:       kubeClient,
		deploymentClient: deploymentClient,
		// deploymentIndexer: deploymentInformer.Informer().GetIndexer(),
		deploymentLister: deploymentInformer.Lister(),
		syncFuncs:        syncFuncs,
	}

	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.enqueue(obj) },
		UpdateFunc: func(_, obj interface{}) { c.enqueue(obj) },
	})

	return c
}

type Controller struct {
	queue            workqueue.RateLimitingInterface
	clusterLister    clusterlisters.ClusterLister
	kubeClient       kubernetes.ClusterInterface
	deploymentClient appsv1client.DeploymentsGetter
	// deploymentIndexer cache.Indexer
	deploymentLister appsv1lister.DeploymentLister
	syncFuncs        []cache.InformerSynced
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.ObjectKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.Add(key)
}

func (c *Controller) Start(ctx context.Context, numThreads int) {
	defer c.queue.ShutDown()

	klog.Infof("Starting workers")
	defer klog.Infof("Stopping workers")

	if !cache.WaitForNamedCacheSync("deployment-controller", ctx.Done(), c.syncFuncs...) {
		klog.Errorf("deployment-controller's caches did not get synced - will not run")
		return
	}

	for i := 0; i < numThreads; i++ {
		go wait.Until(func() { c.startWorker(ctx) }, time.Second, ctx.Done())
	}

	<-ctx.Done()
}

func (c *Controller) startWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	// Wait until there is a new item in the working queue
	k, quit := c.queue.Get()
	if quit {
		return false
	}

	// No matter what, tell the queue we're done with this key, to unblock
	// other workers.
	defer c.queue.Done(k)

	key, err := cache.DecodeKeyFunc(k.(string))
	if err != nil {
		runtime.HandleError(fmt.Errorf("%q controller unable to decode key %s: %w", controllerName, k, err))
		return true
	}

	ctx = cache.NewSyncContext(ctx, key)

	// c.clusterLister.Filter(cache.ListAllIndex, kcp.FilterValues(ctx)}/* lcluster name */)

	if err := c.process(ctx, key); err != nil {
		runtime.HandleError(fmt.Errorf("%q controller failed to sync %q, err: %w", controllerName, key, err))
		c.queue.AddRateLimited(k)
		return true
	}
	c.queue.Forget(key)
	return true
}

func (c *Controller) process(ctx context.Context, key cache.QueueKey) error {
	deployment, err := c.deploymentLister.Deployments(key.Namespace()).GetWithContext(ctx, key.Name())
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	current := deployment.DeepCopy()
	previous := deployment

	if err := c.reconcile(ctx, current); err != nil {
		return err
	}

	// If the object being reconciled changed as a result, update it.
	if !equality.Semantic.DeepEqual(previous, current) {
		// _, uerr := c.kubeClient.Cluster(current.ClusterName).AppsV1().Deployments(current.Namespace).Update(ctx, current, metav1.UpdateOptions{})
		_, uerr := c.deploymentClient.Deployments(current.Namespace).Update(ctx, current, metav1.UpdateOptions{})
		return uerr
	}

	return err
}
