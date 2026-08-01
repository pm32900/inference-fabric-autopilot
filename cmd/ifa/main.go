package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

const defaultBase = "http://localhost:8080"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	base := os.Getenv("IFA_URL")
	if base == "" {
		base = defaultBase
	}

	switch os.Args[1] {
	case "telemetry":
		runTelemetry(base)
	case "recommendations":
		runRecommendations(base)
	case "workloads":
		runWorkloads(base)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: ifa <command>")
	fmt.Println("Commands:")
	fmt.Println("  telemetry         Show latest telemetry per workload")
	fmt.Println("  recommendations   Show active recommendations")
	fmt.Println("  workloads         Show kubernetes discovered workloads")
	fmt.Println()
	fmt.Println("Env:")
	fmt.Println("IFA_URL.            Control plane base URL (default: http://localhost:8080)")
}

// telemetry

func runTelemetry(base string) {
	var snaps []telemetry.Snapshot
	mustGet(base+"/telemetry", &snaps)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKLOAD\tNAMESPACE\tRUNTIME\tGPU%\tMEM%\tQUEUE\tP95ms\tERR%\tTIME")
	for _, s := range snaps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%.1f\t%.1f\t%d\t%.0f\t%.2f\t%s\n",
			s.WorkloadName, s.Namespace, s.Runtime,
			s.GPUUtilizationPct, s.GPUMemoryUsedPct, s.QueueDepth,
			s.P95LatencyMs, s.ErrorRatePct,
			s.Timestamp.Local().Format(time.TimeOnly),
		)
	}
	w.Flush()
}

// recommendations
func runRecommendations(base string) {
	var recs []telemetry.Recommendation
	mustGet(base+"/recommendations", &recs)

	if len(recs) == 0 {
		fmt.Println("No active recommendations.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSEVERITY\tWORKLOAD\tTITLE")
	for _, r := range recs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ID, r.Severity, r.WorkloadName, r.Title)
	}
	w.Flush()

	fmt.Println()
	for _, r := range recs {
		fmt.Printf("[%s] %s — %s\n", r.ID, r.WorkloadName, r.Title)
		fmt.Printf("  Explanation:  %s\n", r.Explanation)
		fmt.Printf("  Action:       %s\n", r.SuggestedAction)
		fmt.Printf("  Metric:       %s\n\n", r.RelatedMetric)
	}
}

// workloads

func runWorkloads(base string) {
	var raw []json.RawMessage
	mustGet(base+"/workloads", &raw)

	if len(raw) == 0 {
		fmt.Println("No workloads discovered (k8s watcher may be disabled).")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tNAMESPACE\tRUNTIME\tMODEL\tGPU\tRESTARTS")
	for _, item := range raw {
		var obj map[string]any
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\n",
			strVal(obj, "Name"), strVal(obj, "Namespace"),
			strVal(obj, "Runtime"), strVal(obj, "ModelName"),
			strVal(obj, "GPURequest"), obj["RestartCount"],
		)
	}
	w.Flush()
}

// helpers

func mustGet(url string, out any) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: server returned %d\n", resp.StatusCode)
		os.Exit(1)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		fmt.Fprintf(os.Stderr, "error decoding response: %v\n", err)
		os.Exit(1)
	}
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}

	return "-"
}
