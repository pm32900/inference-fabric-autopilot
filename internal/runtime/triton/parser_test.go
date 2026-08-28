package triton

import (
	"testing"
)

const fullPayload = `
# HELP nv_inference_request_success Number of successful inference requests
# TYPE nv_inference_request_success counter
nv_inference_request_success{model="resnet50",version="1"} 8192
nv_inference_request_success{model="bert-large",version="2"} 3401
# HELP nv_inference_request_failure Number of failed inference requests
# TYPE nv_inference_request_failure counter
nv_inference_request_failure{model="resnet50",version="1"} 12
nv_inference_request_failure{model="bert-large",version="2"} 0
# HELP nv_inference_count Number of inferences performed
# TYPE nv_inference_count counter
nv_inference_count{model="resnet50",version="1"} 8192
nv_inference_count{model="bert-large",version="2"} 3401
# HELP nv_inference_request_duration_us Cumulative request duration in microseconds
# TYPE nv_inference_request_duration_us counter
nv_inference_request_duration_us{model="resnet50",version="1"} 409600000
nv_inference_request_duration_us{model="bert-large",version="2"} 510150000
# HELP nv_inference_queue_duration_us Cumulative queue duration in microseconds
# TYPE nv_inference_queue_duration_us counter
nv_inference_queue_duration_us{model="resnet50",version="1"} 81920000
nv_inference_queue_duration_us{model="bert-large",version="2"} 204060000
# HELP nv_inference_compute_infer_duration_us Cumulative compute duration in microseconds
# TYPE nv_inference_compute_infer_duration_us counter
nv_inference_compute_infer_duration_us{model="resnet50",version="1"} 327680000
nv_inference_compute_infer_duration_us{model="bert-large",version="2"} 306090000
# HELP nv_inference_pending_request_count Pending request count
# TYPE nv_inference_pending_request_count gauge
nv_inference_pending_request_count{model="resnet50",version="1"} 3
nv_inference_pending_request_count{model="bert-large",version="2"} 11
# HELP nv_gpu_utilization GPU utilization (0-100)
# TYPE nv_gpu_utilization gauge
nv_gpu_utilization{gpu_uuid="GPU-aaa"} 78
nv_gpu_utilization{gpu_uuid="GPU-bbb"} 91
# HELP nv_gpu_memory_used_bytes GPU memory used in bytes
# TYPE nv_gpu_memory_used_bytes gauge
nv_gpu_memory_used_bytes{gpu_uuid="GPU-aaa"} 21474836480
nv_gpu_memory_used_bytes{gpu_uuid="GPU-bbb"} 38654705664
# HELP nv_gpu_memory_total_bytes GPU total memory in bytes
# TYPE nv_gpu_memory_total_bytes gauge
nv_gpu_memory_total_bytes{gpu_uuid="GPU-aaa"} 42949672960
nv_gpu_memory_total_bytes{gpu_uuid="GPU-bbb"} 42949672960
`

func TestParse_AllModels(t *testing.T) {
	snaps := Parse(fullPayload, "")
	if len(snaps) != 2 {
		t.Fatalf("expected 2 model snapshots, got %d", len(snaps))
	}
}

func TestParse_ModelFilter(t *testing.T) {
	snaps := Parse(fullPayload, "resnet50")
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot for resnet50, got %d", len(snaps))
	}
	s := snaps[0]
	if s.ModelName != "resnet50" {
		t.Errorf("ModelName: got %q, want resnet50", s.ModelName)
	}
	if s.InferenceSuccessTotal != 8192 {
		t.Errorf("InferenceSuccessTotal: got %.0f, want 8192", s.InferenceSuccessTotal)
	}
	if s.InferenceFailureTotal != 12 {
		t.Errorf("InferenceFailureTotal: got %.0f, want 12", s.InferenceFailureTotal)
	}
	if s.PendingRequestCount != 3 {
		t.Errorf("PendingRequestCount: got %d, want 3", s.PendingRequestCount)
	}
	if s.QueueDurationUsTotal != 81920000 {
		t.Errorf("QueueDurationUsTotal: got %.0f, want 81920000", s.QueueDurationUsTotal)
	}
}

func TestParse_GPUMetricsAggregatedAsMax(t *testing.T) {
	snaps := Parse(fullPayload, "resnet50")
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	s := snaps[0]
	// max GPU util across GPU-aaa(78) and GPU-bbb(91) = 91
	if s.GPUUtilizationPct != 91 {
		t.Errorf("GPUUtilizationPct: got %.1f, want 91 (max across GPUs)", s.GPUUtilizationPct)
	}
	// max GPU mem used = 38654705664 (GPU-bbb)
	if s.GPUMemoryUsedBytes != 38654705664 {
		t.Errorf("GPUMemoryUsedBytes: got %.0f, want 38654705664", s.GPUMemoryUsedBytes)
	}
	// GPUMemoryUsedPct = 38654705664/42949672960*100 ≈ 90.0
	if s.GPUMemoryUsedPct < 89.9 || s.GPUMemoryUsedPct > 90.1 {
		t.Errorf("GPUMemoryUsedPct: got %.4f, want ~90.0", s.GPUMemoryUsedPct)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	snaps := Parse("", "")
	if len(snaps) != 0 {
		t.Errorf("expected empty slice, got %d", len(snaps))
	}
}

func TestParse_CommentsOnly(t *testing.T) {
	input := `# HELP nv_inference_request_success Number of successful inference requests
# TYPE nv_inference_request_success counter
`
	snaps := Parse(input, "")
	if len(snaps) != 0 {
		t.Errorf("expected empty slice for comments-only input, got %d", len(snaps))
	}
}

func TestParse_MalformedLinesSkipped(t *testing.T) {
	input := `
nv_inference_request_success{model="resnet50",version="1"} notanumber
nv_inference_request_success{model="bert-large",version="1"} 500
`
	snaps := Parse(input, "")
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot (malformed line skipped), got %d", len(snaps))
	}
	if snaps[0].ModelName != "bert-large" {
		t.Errorf("expected bert-large, got %q", snaps[0].ModelName)
	}
}

func TestParse_FilterNonexistentModel(t *testing.T) {
	snaps := Parse(fullPayload, "nonexistent-model")
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots for unknown model, got %d", len(snaps))
	}
}
