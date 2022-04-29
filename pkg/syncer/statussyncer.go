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

package syncer

import (
	"context"
	"encoding/json"

	"github.com/kcp-dev/apimachinery/pkg/logicalcluster"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	workloadv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/workload/v1alpha1"
)

func deepEqualFinalizersAndStatus(oldObj, newObj interface{}) bool {
	oldUnstrob, isOldObjUnstructured := oldObj.(*unstructured.Unstructured)
	newUnstrob, isNewObjUnstructured := newObj.(*unstructured.Unstructured)
	if !isOldObjUnstructured || !isNewObjUnstructured || oldObj == nil || newObj == nil {
		return false
	}

	newFinalizers := newUnstrob.GetFinalizers()
	oldFinalizers := oldUnstrob.GetFinalizers()

	newStatus := newUnstrob.UnstructuredContent()["status"]
	oldStatus := oldUnstrob.UnstructuredContent()["status"]

	return equality.Semantic.DeepEqual(oldFinalizers, newFinalizers) && equality.Semantic.DeepEqual(oldStatus, newStatus)
}

const statusSyncerAgent = "kcp#status-syncer/v0.0.0"

func NewStatusSyncer(from, to *rest.Config, gvrs []string, kcpClusterName logicalcluster.LogicalCluster, pclusterID string) (*Controller, error) {
	from = rest.CopyConfig(from)
	from.UserAgent = statusSyncerAgent
	to = rest.CopyConfig(to)
	to.UserAgent = statusSyncerAgent

	fromClient := dynamic.NewForConfigOrDie(from)
	toClients, err := dynamic.NewClusterForConfig(to)
	if err != nil {
		return nil, err
	}
	toClient := toClients.Cluster(kcpClusterName)

	// Register the default mutators
	mutatorsMap := getDefaultMutators(from)

	return New(kcpClusterName, pclusterID, fromClient, toClient, SyncUp, gvrs, pclusterID, mutatorsMap)
}

func (c *Controller) deleteFromUpstream(ctx context.Context, gvr schema.GroupVersionResource, upstreamNamespace, name string) error {
	return c.removeUpstreamSyncerOwnership(ctx, gvr, upstreamNamespace, name)
}

func (c *Controller) removeUpstreamSyncerOwnership(ctx context.Context, gvr schema.GroupVersionResource, upstreamNamespace, name string) error {
	upstreamObj, err := c.toClient.Resource(gvr).Namespace(upstreamNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		return nil
	}

	// TODO(jmprusi): This check will need to be against "GetDeletionTimestamp()" when using the syncer virtual  workspace.
	if upstreamObj.GetAnnotations()[workloadv1alpha1.InternalClusterDeletionTimestampAnnotationPrefix+c.pcluster] == "" {
		// Do nothing: the object should not be deleted anymore for this location on the KCP side
		return nil
	}

	// Remove the syncer finalizer.
	currentFinalizers := upstreamObj.GetFinalizers()
	desiredFinalizers := []string{}
	for _, finalizer := range currentFinalizers {
		if finalizer != syncerFinalizerNamePrefix+c.pcluster {
			desiredFinalizers = append(desiredFinalizers, finalizer)
		}
	}
	upstreamObj.SetFinalizers(desiredFinalizers)

	//  TODO(jmprusi): This code block will be handled by the syncer virtual workspace, so we can remove it once
	//                 the virtual workspace syncer is integrated
	//  - Begin -
	// Clean up the status annotation and the locationDeletionAnnotation.
	annotations := upstreamObj.GetAnnotations()
	delete(annotations, workloadv1alpha1.InternalClusterStatusAnnotationPrefix+c.pcluster)
	delete(annotations, workloadv1alpha1.InternalClusterDeletionTimestampAnnotationPrefix+c.pcluster)
	delete(annotations, workloadv1alpha1.InternalClusterStatusAnnotationPrefix+c.pcluster)
	upstreamObj.SetAnnotations(annotations)

	// remove the cluster label.
	upstreamLabels := upstreamObj.GetLabels()
	delete(upstreamLabels, workloadv1alpha1.InternalClusterResourceStateLabelPrefix+c.pcluster)
	upstreamObj.SetLabels(upstreamLabels)
	// - End of block to be removed once the virtual workspace syncer is integrated -

	if _, err := c.toClient.Resource(gvr).Namespace(upstreamObj.GetNamespace()).Update(ctx, upstreamObj, metav1.UpdateOptions{}); err != nil {
		klog.Errorf("Failed updating after removing the finalizers of resource %s|%s/%s: %v", c.upstreamClusterName, upstreamNamespace, upstreamObj.GetName(), err)
		return err
	}
	klog.Infof("Updated resource %s|%s/%s after removing the finalizers", c.upstreamClusterName, upstreamNamespace, upstreamObj.GetName())
	return nil
}

func (c *Controller) updateStatusInUpstream(ctx context.Context, gvr schema.GroupVersionResource, upstreamNamespace string, downstreamObj *unstructured.Unstructured) error {
	upstreamObj := downstreamObj.DeepCopy()
	upstreamObj.SetUID("")
	upstreamObj.SetResourceVersion("")
	upstreamObj.SetNamespace(upstreamNamespace)

	// Run name transformations on upstreamObj
	transformName(upstreamObj, SyncUp)

	name := upstreamObj.GetName()
	downstreamStatus, statusExists, err := unstructured.NestedFieldCopy(upstreamObj.UnstructuredContent(), "status")
	if err != nil {
		return err
	} else if !statusExists {
		klog.Infof("Resource doesn't contain a status. Skipping updating status of resource %s|%s/%s from pcluster namespace %s", c.upstreamClusterName, upstreamNamespace, name, downstreamObj.GetNamespace())
		return nil
	}

	existing, err := c.toClient.Resource(gvr).Namespace(upstreamNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Getting resource %s/%s: %v", upstreamNamespace, name, err)
		return err
	}

	// Run any transformations on the object before we update the status on kcp.
	if mutator, ok := c.mutators[gvr]; ok {
		if err := mutator(upstreamObj); err != nil {
			return err
		}
	}

	// TODO: verify that we really only update status, and not some non-status fields in ObjectMeta.
	//       I believe to remember that we had resources where that happened.

	upstreamObj.SetResourceVersion(existing.GetResourceVersion())

	if advancedSchedulingFeatureEnabled {
		newUpstream := existing.DeepCopy()
		statusAnnotationValue, err := json.Marshal(downstreamStatus)
		if err != nil {
			return err
		}
		newUpstreamAnnotations := newUpstream.GetAnnotations()
		newUpstreamAnnotations[workloadv1alpha1.InternalClusterStatusAnnotationPrefix+c.pcluster] = string(statusAnnotationValue)
		newUpstream.SetAnnotations(newUpstreamAnnotations)

		if _, err := c.toClient.Resource(gvr).Namespace(upstreamNamespace).Update(ctx, newUpstream, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("Failed updating location status annotation of resource %s|%s/%s from pcluster namespace %s: %v", c.upstreamClusterName, upstreamNamespace, upstreamObj.GetName(), downstreamObj.GetNamespace(), err)
			return err
		}
		klog.Infof("Updated status of resource %s|%s/%s from pcluster namespace %s", c.upstreamClusterName, upstreamNamespace, upstreamObj.GetName(), downstreamObj.GetNamespace())
	} else {
		if _, err := c.toClient.Resource(gvr).Namespace(upstreamNamespace).UpdateStatus(ctx, upstreamObj, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("Failed updating status of resource %s|%s/%s from pcluster namespace %s: %v", c.upstreamClusterName, upstreamNamespace, upstreamObj.GetName(), downstreamObj.GetNamespace(), err)
			return err
		}
		klog.Infof("Updated status of resource %s|%s/%s from pcluster namespace %s", c.upstreamClusterName, upstreamNamespace, upstreamObj.GetName(), downstreamObj.GetNamespace())
	}
	return nil
}
