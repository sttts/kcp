package syncer

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
