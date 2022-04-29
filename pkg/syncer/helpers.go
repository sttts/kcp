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

import (
	"strings"

	workloadv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/workload/v1alpha1"
)

func LocationDeletionAnnotationName(workloadClusterName string) string {
	return workloadv1alpha1.InternalClusterDeletionTimestampAnnotationPrefix + workloadClusterName
}

func LocationFinalizersAnnotationName(workloadClusterName string) string {
	return workloadv1alpha1.ClusterFinalizerAnnotationPrefix + workloadClusterName
}

func LocationStatusAnnotationName(workloadClusterName string) string {
	return workloadv1alpha1.InternalClusterStatusAnnotationPrefix + workloadClusterName
}

func LocationSpecDiffAnnotationName(workloadClusterName string) string {
	return workloadv1alpha1.InternalClusterStatusAnnotationPrefix + workloadClusterName
}

func SyncerFinalizerName(workloadClusterName string) string {
	return "workloads.kcp.dev/syncer-" + workloadClusterName
}

func WorkloadClusterLabelName(workloadClusterName string) string {
	return workloadv1alpha1.InternalClusterStateLabelPrefix + workloadClusterName
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
