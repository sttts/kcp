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

package syncer

import "strings"

func LocationDeletionAnnotationName(workloadClusterName string) string {
	return "deletion.internal.workloads.kcp.dev/" + workloadClusterName
}

func LocationFinalizersAnnotationName(workloadClusterName string) string {
	return "finalizers.workloads.kcp.dev/" + workloadClusterName
}

func LocationStatusAnnotationName(workloadClusterName string) string {
	return "experimental.status.workloads.kcp.dev/" + workloadClusterName
}

func LocationSpecDiffAnnotationName(workloadClusterName string) string {
	return "experimental.specdiff.workloads.kcp.dev/" + workloadClusterName
}

func SyncerFinalizerName(workloadClusterName string) string {
	return "workloads.kcp.dev/syncer-" + workloadClusterName
}

func WorkloadClusterLabelName(workloadClusterName string) string {
	return "cluster.internal.workloads.kcp.dev/" + workloadClusterName
}

func GetAssignedWorkloadCluster(labels map[string]string) string {
	for k, v := range labels {
		if strings.HasPrefix(k, WorkloadClusterLabelName("")) && v == "Sync" {
			return strings.TrimPrefix(k, WorkloadClusterLabelName(""))
		}
	}
	return ""
}

func GetAssignedWorkloadClusters(labels map[string]string) []string {
	var clusters []string
	for k, v := range labels {
		if strings.HasPrefix(k, WorkloadClusterLabelName("")) && v == "Sync" {
			clusters = append(clusters, strings.TrimPrefix(k, WorkloadClusterLabelName("")))
		}
	}
	return clusters
}
