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

const (
	// InternalClusterDeletionTimestampAnnotationPrefix is the prefix of the annotation
	//
	//   deletion.internal.workloads.kcp.dev/<workload-cluster-name>
	//
	// on upstream resources storing the timestamp when the workload cluster resource
	// state was changed to "Delete". The syncer will see this timestamp as the deletion
	// timestamp of the object.
	//
	// The format is RFC3339.
	//
	// TODO(sttts): use workload-cluster-uid instead of workload-cluster-name
	InternalClusterDeletionTimestampAnnotationPrefix = "deletion.internal.workloads.kcp.dev/"

	// ClusterFinalizerAnnotationPrefix is the prefix of the annotation
	//
	//   finalizers.workloads.kcp.dev/<workload-cluster-name>
	//
	// on upstream resources storing a comma-separated list of finalizer names that are set on
	// the workload cluster resource in the view of the syncer. This blocks the deletion of the
	// resource on that workload cluster. External (custom) controllers can set this annotation
	// create back-pressure on the resource.
	//
	// TODO(sttts): use workload-cluster-uid instead of workload-cluster-name
	ClusterFinalizerAnnotationPrefix = "finalizers.workloads.kcp.dev/"

	// InternalWorkloadClusterStateLabelPrefix is the prefix of the label
	//
	//   state.internal.workloads.kcp.dev/<workload-cluster-name>
	//
	// on upstream resources storing the state of the workload cluster syncer state machine.
	// The workload controllers will set this label and the syncer will react and drive the
	// life-cycle of the synced objects on the workload cluster.
	//
	// The format is a string, namely:
	// - "": the object is assigned, but the syncer will ignore the object. A coordination
	//       controller will have to set the value to "Sync" after initializion in order to
	//       start the sync process.
	// - "Sync": the object is assigned and the syncer will start the sync process.
	// - "Delete": the object is assigned and the syncer will start the deletion procedure.
	//             Deletion is blocked if finalizers.workloads.kcp.dev/<workload-cluster-name>
	//             is non-empty.
	//
	// The workload controllers will consider the object deleted from the workload cluster when
	// the label is removed. They then set the placement state to "Unbound".
	InternalWorkloadClusterStateLabelPrefix = "state.internal.workloads.kcp.dev/"

	// InternalClusterStatusAnnotationPrefix is the prefix of the annotation
	//
	//   experimental.status.workloads.kcp.dev/<workload-cluster-name>
	//
	// on upstream resources storing the status of the downstream resource per workload cluster.
	// Note that this is experimental and will disappear in the future without prior notice. It
	// is used temporarily in the case that a resource is scheduled to multiple workload clusters.
	//
	// The format is JSON.
	InternalClusterStatusAnnotationPrefix = "experimental.status.workloads.kcp.dev/"
)
