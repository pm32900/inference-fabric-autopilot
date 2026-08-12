package dcgm

import (
	"testing"
)

func TestParse_FullPayload(t *testing.T) {
	input := `
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization (in %)
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-aaa"} 87
DCGM_FI_DEV_GPU_UTIL{gpu="1",UUID="GPU-bbb"} 43
# HELP DCGM_FI_DEV_MEM_COPY_UTIL Memory bandwidth utilization (in %)
# TYPE DCGM_FI_DEV_MEM_COPY_UTIL gauge
DCGM_FI_DEV_MEM_COPY_UTIL{gpu="0",UUID="GPU-aaa"} 61
DCGM_FI_DEV_MEM_COPY_UTIL{gpu="1",UUID="GPU-bbb"} 30
# HELP DCGM_FI_DEV_FB_USED Framebuffer memory used (in MiB)
# TYPE DCGM_FI_DEV_FB_USED gauge
DCGM_FI_DEV_FB_USED{gpu="0",UUID="GPU-aaa"} 38912
DCGM_FI_DEV_FB_USED{gpu="1",UUID="GPU-bbb"} 20480
# HELP DCGM_FI_DEV_FB_FREE Framebuffer memory free (in MiB)
# TYPE DCGM_FI_DEV_FB_FREE gauge
DCGM_FI_DEV_FB_FREE{gpu="0",UUID="GPU-aaa"} 1280
DCGM_FI_DEV_FB_FREE{gpu="1",UUID="GPU-bbb"} 19712
# HELP DCGM_FI_DEV_SM_CLOCK SM clock frequency (in MHz)
# TYPE DCGM_FI_DEV_SM_CLOCK gauge
DCGM_FI_DEV_SM_CLOCK{gpu="0",UUID="GPU-aaa"} 1410
DCGM_FI_DEV_SM_CLOCK{gpu="1",UUID="GPU-bbb"} 1215
# HELP DCGM_FI_DEV_GPU_TEMP GPU temperature (in C)
# TYPE DCGM_FI_DEV_GPU_TEMP gauge
DCGM_FI_DEV_GPU_TEMP{gpu="0",UUID="GPU-aaa"} 74
DCGM_FI_DEV_GPU_TEMP{gpu="1",UUID="GPU-bbb"} 61
`
	snaps := Parse(input)

	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	gpu0 := snaps[0]
	if gpu0.GPUIndex != 0 {
		t.Errorf("gpu0 index: got %d, want 0", gpu0.GPUIndex)
	}
	if gpu0.GPUUtilPct != 87 {
		t.Errorf("gpu0 GPUUtilPct: got %.1f, want 87", gpu0.GPUUtilPct)
	}
	if gpu0.FBUsedMiB != 38912 {
		t.Errorf("gpu0 FBUsedMiB: got %.0f, want 38912", gpu0.FBUsedMiB)
	}
	if gpu0.FBTotalMiB != 40192 {
		t.Errorf("gpu0 FBTotalMiB: got %.0f, want 40192 (38912+1280)", gpu0.FBTotalMiB)
	}
	// FBUsedPct = 38912/40192*100 ≈ 96.82
	if gpu0.FBUsedPct < 96.8 || gpu0.FBUsedPct > 96.9 {
		t.Errorf("gpu0 FBUsedPct: got %.4f, want ~96.82", gpu0.FBUsedPct)
	}
	if gpu0.TempCelsius != 74 {
		t.Errorf("gpu0 TempCelsius: got %.0f, want 74", gpu0.TempCelsius)
	}

	gpu1 := snaps[1]
	if gpu1.GPUIndex != 1 {
		t.Errorf("gpu1 index: got %d, want 1", gpu1.GPUIndex)
	}
	if gpu1.GPUUtilPct != 43 {
		t.Errorf("gpu1 GPUUtilPct: got %.1f, want 43", gpu1.GPUUtilPct)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	snaps := Parse("")
	if len(snaps) != 0 {
		t.Errorf("expected empty slice, got %d snapshots", len(snaps))
	}
}

func TestParse_CommentsOnly(t *testing.T) {
	input := `# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
`
	snaps := Parse(input)
	if len(snaps) != 0 {
		t.Errorf("expected empty slice for comments-only input, got %d", len(snaps))
	}
}

func TestParse_MissingFBFree(t *testing.T) {
	// If FB_FREE is absent, FBTotalMiB = FBUsedMiB and FBUsedPct = 100
	input := `DCGM_FI_DEV_FB_USED{gpu="0",UUID="GPU-aaa"} 16384`
	snaps := Parse(input)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].FBTotalMiB != 16384 {
		t.Errorf("FBTotalMiB: got %.0f, want 16384", snaps[0].FBTotalMiB)
	}
	if snaps[0].FBUsedPct != 100.0 {
		t.Errorf("FBUsedPct: got %.1f, want 100.0 when FB_FREE is absent", snaps[0].FBUsedPct)
	}
}

func TestParse_SingleGPU_FBUsedPctFiftyPercent(t *testing.T) {
	input := `
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-xyz"} 55
DCGM_FI_DEV_FB_USED{gpu="0",UUID="GPU-xyz"} 8192
DCGM_FI_DEV_FB_FREE{gpu="0",UUID="GPU-xyz"} 8192
`
	snaps := Parse(input)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].FBUsedPct != 50.0 {
		t.Errorf("FBUsedPct: got %.1f, want 50.0", snaps[0].FBUsedPct)
	}
	if snaps[0].GPUUtilPct != 55 {
		t.Errorf("GPUUtilPct: got %.1f, want 55", snaps[0].GPUUtilPct)
	}
}

func TestParse_MalformedLinesSkipped(t *testing.T) {
	input := `
DCGM_FI_DEV_GPU_UTIL{gpu="0"} notanumber
DCGM_FI_DEV_GPU_UTIL{gpu="1",UUID="GPU-bbb"} 72
`
	snaps := Parse(input)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot (malformed line skipped), got %d", len(snaps))
	}
	if snaps[0].GPUIndex != 1 {
		t.Errorf("expected GPU index 1, got %d", snaps[0].GPUIndex)
	}
	if snaps[0].GPUUtilPct != 72 {
		t.Errorf("GPUUtilPct: got %.1f, want 72", snaps[0].GPUUtilPct)
	}
}
