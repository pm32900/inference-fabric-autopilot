package dcgm

const (
	// MetricGPUUtil is GPU compute util as a percentage
	MetricGPUUtil = "DCGM_FI_DEV_GPU_UTIL"

	// is GPU memory bandwidth util as a percentage
	MetricMemCopyUtil = "DCGM_FI_DEV_MEM_COPY_UTIL"

	// is framebuffer (VRAM) used in MB
	MetricFBUsed = "DCGM_FI_DEV_FB_USED"

	// is framebuffer (VRAM) free in MB
	MetricFBFree = "DCGM_FI_DEV_FB_FREE"

	// is the current SM clock in MHz
	MetricSMClock = "DCGM_FI_DEV_SM_CLOCK"

	// GPU core temp in celsius
	MetricGPUTemp = "DCGM_FI_DEV_GPU_TEMP"
)

// DCGMSnapshot = GPU level metrics for a single GPU device at one point in time
type DCGMSnapshot struct {
	//is the integer device index (from gpu label in metrics)
	GPUIndex int
	// GPU util as percent
	GPUUtilPct float64
	// is memory bandwidth util
	MemCopyUtilPct float64
	// is framebuffer memory used in MB
	FBUsedMiB float64
	// is framebuffer memory free in MB
	FBFreeMiB float64
	// total = FBUsedMiB + FBFreeMiB
	FBTotalMiB float64
	// FBUsedPct is derived: (FBUsedMiB / FBTotalMiB) * 100
	FBUsedPct float64
	// is the current streaming multiprocessor clock in MHz
	SMClockMHZ float64
	// is the GPU core temperature in celsius
	TempCelsius float64
}
