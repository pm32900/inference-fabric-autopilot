package dcgm

import (
	"math"
	"testing"
)

const twoGPUs = `# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization (in %).
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-a",device="nvidia0",Hostname="node-1"} 94
DCGM_FI_DEV_GPU_UTIL{gpu="1",UUID="GPU-b",device="nvidia1",Hostname="node-1"} 3
# TYPE DCGM_FI_DEV_FB_USED gauge
DCGM_FI_DEV_FB_USED{gpu="0",UUID="GPU-a"} 76000
DCGM_FI_DEV_FB_USED{gpu="1",UUID="GPU-b"} 1024
# TYPE DCGM_FI_DEV_FB_FREE gauge
DCGM_FI_DEV_FB_FREE{gpu="0",UUID="GPU-a"} 4000
DCGM_FI_DEV_FB_FREE{gpu="1",UUID="GPU-b"} 78976
# TYPE DCGM_FI_DEV_GPU_TEMP gauge
DCGM_FI_DEV_GPU_TEMP{gpu="0",UUID="GPU-a"} 78
DCGM_FI_DEV_GPU_TEMP{gpu="1",UUID="GPU-b"} 41
`

func TestParseOrdersDevicesByIndex(t *testing.T) {
	devices, err := Parse(twoGPUs)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].Index != 0 || devices[1].Index != 1 {
		t.Errorf("devices out of order: %d, %d", devices[0].Index, devices[1].Index)
	}
	if devices[0].UtilizationPct.Value != 94 || devices[1].UtilizationPct.Value != 3 {
		t.Errorf("utilisation = %v, %v", devices[0].UtilizationPct, devices[1].UtilizationPct)
	}
	if devices[0].TemperatureC.Value != 78 {
		t.Errorf("temperature = %v", devices[0].TemperatureC)
	}
}

// DCGM reports used and free framebuffer, never a total, so the percentage has
// to be derived from both.
func TestMemoryPercentageIsDerivedFromUsedAndFree(t *testing.T) {
	devices, err := Parse(twoGPUs)
	if err != nil {
		t.Fatal(err)
	}
	if got := devices[0].MemoryUsedPct.Value; math.Abs(got-95) > 1e-9 {
		t.Errorf("device 0 memory = %v%%, want 95", got)
	}
	if got := devices[1].MemoryUsedPct.Value; math.Abs(got-1.28) > 1e-9 {
		t.Errorf("device 1 memory = %v%%, want 1.28", got)
	}
}

// Reporting a used-MiB figure as a percentage would be an order-of-magnitude
// error, so a device missing either half stays unmeasured.
func TestMemoryPercentageNeedsBothHalves(t *testing.T) {
	devices, err := Parse(`DCGM_FI_DEV_FB_USED{gpu="0"} 40000` + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices", len(devices))
	}
	if devices[0].MemoryUsedPct.OK {
		t.Errorf("memory percentage = %v derived without a free-memory reading", devices[0].MemoryUsedPct)
	}
	if !devices[0].MemoryUsedMiB.OK {
		t.Error("the raw used-memory reading was lost")
	}
}

func TestNoGPUsIsNotAnError(t *testing.T) {
	for _, payload := range []string{
		"",
		"# just comments\n",
		"some_other_exporter_metric 5\n",
		"DCGM_FI_DEV_GPU_UTIL 94\n", // no gpu label: not a per-device series
	} {
		devices, err := Parse(payload)
		if err != nil {
			t.Errorf("payload %q: %v", payload, err)
		}
		if len(devices) != 0 {
			t.Errorf("payload %q produced %d devices", payload, len(devices))
		}
	}
}

func TestUnparseableGPULabelIsSkipped(t *testing.T) {
	devices, err := Parse(`DCGM_FI_DEV_GPU_UTIL{gpu="not-a-number"} 50
DCGM_FI_DEV_GPU_UTIL{gpu="2"} 60
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Index != 2 {
		t.Errorf("got %+v, want only device 2", devices)
	}
}
