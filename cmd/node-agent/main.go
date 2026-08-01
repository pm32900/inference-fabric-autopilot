package main

import (
	"log"
	"time"
)

func main() {
	log.Println("Node agent staring...")
	log.Println("Phase 1: stub mode, no real metric collection yet")
	log.Println("In a future phase this agent will:")
	log.Println("	- Read GPU metrics from NVIDIA DCGM or /proc")
	log.Println("  - Scrape inference runtime metrics (vLLM, Triton, Ollama)")
	log.Println("  - Push snapshots to the control plane via HTTP or gRPC")

	// simulate the agebt "running" by logging a heartbeat every 10 seconds
	// in phase 2 this loop will collect and ship real telemetry
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case t := <-ticker.C:
			log.Printf("[heartbeat] node-agent alive at %s — would collect metrics here", t.Format(time.RFC3339))
		}
	}
}
