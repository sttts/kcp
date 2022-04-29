/*
Copyright 2022 The KCP Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceState string

var (
	// ResourceStatePending is the initial state of a resource after placement onto
	// workload cluster. Either some workload controller or some external coordination
	// controller will set this to "Sync" when the resource is ready to be synced.
	ResourceStatePending ResourceState = ""
	// ResourceStateSync is the state of a resource when it is synced to the workload cluster.
	ResourceStateSync ResourceState = "Sync"
	// ResourceStateDelete is the state of a resource when it should be deleted from the
	// workload cluster. Deletion is blocked as long as finalizers.workloads.kcp.dev/<workload-cluster-name>
	// is present and non-empty.
	ResourceStateDelete ResourceState = "Delete"
)

// GetResourceState returns the state of the resource for the given workload cluster, and
// whether the state value is a valid state. A missing label is considered invalid.
func GetResourceState(obj metav1.Object, cluster string) (state ResourceState, valid bool) {
	value, found := obj.GetLabels()[InternalWorkloadClusterStateLabelPrefix+cluster]
	return ResourceState(value), found && (value == "" || ResourceState(value) == ResourceStateSync || ResourceState(value) == ResourceStateDelete)
}
