package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// WorkloadStore holds the latest discovered workloads from Kubernetes
// It is safe to read from multiple goroutines
type WorkloadStore struct {
	mu   sync.RWMutex
	data map[string]*WorkloadInfo // key = namespace/name
}

func NewWorkloadStore() *WorkloadStore {
	return &WorkloadStore{data: make(map[string]*WorkloadInfo)}
}

// Set overwrites the entry for a workload
func (ws *WorkloadStore) Set(w *WorkloadInfo) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.data[ws.key(w.Namespace, w.Name)] = w
}

func (ws *WorkloadStore) All() []*WorkloadInfo {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	result := make([]*WorkloadInfo, 0, len(ws.data))
	for _, w := range ws.data {
		result = append(result, w)
	}
	return result
}

func (ws *WorkloadStore) key(ns, name string) string {
	return fmt.Sprintf("%s/%s", ns, name)
}

// Watcher polls Kubernetes for pods and deployments on a fixed interval.
// In a future phase this can be replaced with an informer/watch for efficiency.

type Watcher struct {
	client    *kubernetes.Clientset
	namespace string
	store     *WorkloadStore
	interval  time.Duration
}

// NewWather builds a watcher. It tries in-cluster config first when running inside a pod
// then falls back to the local kubeconfig
func NewWatcher(namespace string, store *WorkloadStore, interval time.Duration) (*Watcher, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		// local kubeconfig
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("building kubeconfig: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building kubernetes client: %w", err)
	}

	return &Watcher{
		client:    client,
		namespace: namespace,
		store:     store,
		interval:  interval,
	}, nil
}

// Start launches the watch loop in a background goroutine.
func (w *Watcher) Start(ctx context.Context) {
	go func() {
		for {
			if err := w.sync(ctx); err != nil {
				fmt.Printf("warn: k8s sync error: %v\n", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(w.interval):
			}
		}
	}()
}

// sync fetches pods from the configured namespace and upserts them into the store.
func (w *Watcher) sync(ctx context.Context) error {
	pods, err := w.client.CoreV1().Pods(w.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}

	for _, pod := range pods.Items {
		info := podToWorkloadInfo(pod)
		w.store.Set(info)
	}
	return nil
}

// podToWorkloadInfo extracts WorkloadInfo from a pod.
// It looks for well-known labels to identify the runtime and model.
func podToWorkloadInfo(pod corev1.Pod) *WorkloadInfo {
	labels := pod.Labels

	// look for common inference runtime labels
	runtime := labels["inference.io/runtime"]
	if runtime == "" {
		runtime = labels["app"] // fallback to app label
	}

	model := labels["inference.io/model"]

	// sum restart counts across all containers
	var restarts int32
	var gpuReq string
	var cpuReq, memReq string

	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}
	for _, c := range pod.Spec.Containers {
		if c.Resources.Requests != nil {
			if v, ok := c.Resources.Requests["nvidia.com/gpu"]; ok {
				gpuReq = v.String()
			}
			if v, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuReq = v.String()
			}
			if v, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memReq = v.String()
			}
		}
	}

	return &WorkloadInfo{
		Name:          pod.Name,
		Namespace:     pod.Namespace,
		Runtime:       runtime,
		ModelName:     model,
		NodeName:      pod.Spec.NodeName,
		Labels:        labels,
		CPURequest:    cpuReq,
		MemoryRequest: memReq,
		GPURequest:    gpuReq,
		RestartCount:  restarts,
		LastUpdated:   time.Now(),
	}
}
