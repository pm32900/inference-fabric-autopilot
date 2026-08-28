package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockServer starts a local HTTP server that mimics the Ollama REST API.
// It returns the server and a teardown function.
func mockServer(t *testing.T, psModels []PSModel, generateResp GenerateResponse) (*httptest.Server, func()) {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PSResponse{Models: psModels})
	})

	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generateResp)
	})

	srv := httptest.NewServer(mux)
	return srv, srv.Close
}

func TestListRunning_ReturnsModels(t *testing.T) {
	models := []PSModel{
		{Name: "llama3:8b", Size: 4831838208, SizeVRAM: 4294967296, ExpiresAt: "2026-01-01T00:00:00Z", Details: Details{NumGPU: 32, NumCPU: 0}},
		{Name: "nomic-embed-text", Size: 274877906, SizeVRAM: 274877906, ExpiresAt: "", Details: Details{NumGPU: 16, NumCPU: 0}},
	}

	srv, teardown := mockServer(t, models, GenerateResponse{})
	defer teardown()

	client := NewClient(srv.URL)
	got, err := client.ListRunning(context.Background())
	if err != nil {
		t.Fatalf("ListRunning error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 models, got %d", len(got))
	}
	if got[0].Name != "llama3:8b" {
		t.Errorf("model[0] name: got %q, want llama3:8b", got[0].Name)
	}
	if got[1].SizeVRAM != 274877906 {
		t.Errorf("model[1] SizeVRAM: got %d, want 274877906", got[1].SizeVRAM)
	}
}

func TestListRunning_EmptyServer(t *testing.T) {
	srv, teardown := mockServer(t, []PSModel{}, GenerateResponse{})
	defer teardown()

	client := NewClient(srv.URL)
	got, err := client.ListRunning(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 models, got %d", len(got))
	}
}

func TestSnapshot_PopulatesCounters(t *testing.T) {
	model := PSModel{
		Name:     "llama3:8b",
		Size:     4831838208,
		SizeVRAM: 4294967296,
		Details:  Details{NumGPU: 32},
	}
	genResp := GenerateResponse{
		Model:                "llama3:8b",
		PromptEvalCount:      128,
		EvalCount:            512,
		PromptEvalDurationNs: 250000000,
		EvalDurationNs:       1500000000,
		TotalDurationNs:      1800000000,
	}

	srv, teardown := mockServer(t, []PSModel{model}, genResp)
	defer teardown()

	client := NewClient(srv.URL)
	snap, err := client.Snapshot(context.Background(), model)
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}

	if snap.ModelName != "llama3:8b" {
		t.Errorf("ModelName: got %q, want llama3:8b", snap.ModelName)
	}
	if snap.SizeVRAM != 4294967296 {
		t.Errorf("SizeVRAM: got %d, want 4294967296", snap.SizeVRAM)
	}
	if snap.EvalCount != 512 {
		t.Errorf("EvalCount: got %d, want 512", snap.EvalCount)
	}
	if snap.PromptEvalCount != 128 {
		t.Errorf("PromptEvalCount: got %d, want 128", snap.PromptEvalCount)
	}
	if snap.TotalDurationNs != 1800000000 {
		t.Errorf("TotalDurationNs: got %d, want 1800000000", snap.TotalDurationNs)
	}
	if snap.NumGPU != 32 {
		t.Errorf("NumGPU: got %d, want 32", snap.NumGPU)
	}
}

func TestSnapshot_GeneratePingFailure_ReturnsPartial(t *testing.T) {
	// Server that returns 500 for generate — Snapshot should still succeed
	// with zero counters rather than returning an error.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(PSResponse{Models: []PSModel{{Name: "llama3:8b"}}})
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL)
	snap, err := client.Snapshot(context.Background(), PSModel{Name: "llama3:8b", Size: 100})
	if err != nil {
		t.Fatalf("expected no error on generate failure, got: %v", err)
	}
	if snap.EvalCount != 0 {
		t.Errorf("EvalCount should be 0 on generate failure, got %d", snap.EvalCount)
	}
	if snap.ModelName != "llama3:8b" {
		t.Errorf("ModelName should still be set, got %q", snap.ModelName)
	}
}

func TestListRunning_ServerUnreachable(t *testing.T) {
	client := NewClient("http://127.0.0.1:19999") // nothing listening here
	_, err := client.ListRunning(context.Background())
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}
