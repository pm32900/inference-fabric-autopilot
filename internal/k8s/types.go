package k8s

import "time"

// WorkloadInfo holds Kubernetes metadata discovered for one inference workload
// This is populated by the watcher and can be joined with telemetry snapshots
// in the recommender to produce richer recommendations

type WorkloadInfo struct {
	Name          string
	Namespace     string
	Runtime       string
	ModelName     string
	Replicas      int32
	ReadyReplicas int32
	NodeName      string
	Labels        map[string]string
	CPURequest    string
	MemoryRequest string
	GPURequest    string
	RestartCount  int32
	LastUpdated   time.Time
}
