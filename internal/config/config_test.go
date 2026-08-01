package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultStandardMode verifies that the default config is the standard mode with egress enabled and all privacy fields fals
func TestDefaultStandardMode(t *testing.T) {
	cfg := defaults()

	if cfg.DeploymentMode != "standard" {
		t.Errorf("expected deployment_mode 'standard', got %q", cfg.DeploymentMode)
	}
	if !cfg.Egress.Enabled {
		t.Error("Expected egress.enabled true in standard mode defaults")
	}
}

// TestDefaultsPrivacyAllFalse verifies that every privacy field defaults to false
func TestDefaultsPrivacyAllFalse(t *testing.T) {
	cfg := defaults()

	if cfg.Privacy.CollectPromptBodies {
		t.Error("Expected privacy.collect_prompt_bodies false by default")
	}
	if cfg.Privacy.CollectResponseBodies {
		t.Error("Expected privacy.collect_response_bodies false by default")
	}
	if cfg.Privacy.CollectHeaders {
		t.Error("Expected privacy.collect_headers false by default")
	}
	if cfg.Privacy.CollectUserIdentifiers {
		t.Error("Expected privacy.collect_user_identifiers false by default")
	}
}

// TestDefaultsRBACReadOnly verifies that the default RBAC mode is read only
func TestDefaultsRBACReadOnly(t *testing.T) {
	cfg := defaults()

	if cfg.RBAC.Mode != "read-only" {
		t.Errorf("expected rbac.mode 'read-only', got %q", cfg.RBAC.Mode)
	}
}

// TestValidateStandardModeValid verifies default standard config passes validation test
func TestValidateStandardModeValid(t *testing.T) {
	cfg := defaults()

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected standard defaults to pass validation, got %v", err)
	}
}

// TestValidateAirgappedModeValid verifies  that a correct airgapped config passes validation test
func TestValidateAirgappedModeValid(t *testing.T) {
	cfg := defaults()
	cfg.Egress.Enabled = false

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid airgapped config to pass validation, got: %v", err)
	}
}

// TestValidateAirgappedWithEgressEnabledFails verifies that airgapped mode
// with egress enabled is rejected

func TestValidateAirgappedWithEgressEnabledFails(t *testing.T) {
	cfg := defaults()
	cfg.DeploymentMode = "airgapped"
	cfg.Egress.Enabled = true // invalid combination

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for airgapped mode with egress enabled, got nil")
	}
}

// TestValidateInvalidDeploymentMode verifies that an unknown deployment mode is rejected.
func TestValidateInvalidDeploymentMode(t *testing.T) {
	cfg := defaults()
	cfg.DeploymentMode = "cloud" // not a valid value just a placeholder

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for unknown deployment_mode, got nil")
	}
}

// TestValidatePrivacyPromptBodiesFails verifies that enabling prompt collection is rejected
func TestValidatePrivacyPromptBodiesFails(t *testing.T) {
	cfg := defaults()
	cfg.Privacy.CollectPromptBodies = true

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for collect_prompt_bodies true, got nil")
	}
}

// TestValidatePrivacyResponseBodiesFails verifies that enabling response collection is rejected
func TestValidatePrivacyResponseBodiesFails(t *testing.T) {
	cfg := defaults()
	cfg.Privacy.CollectResponseBodies = true

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for collect_response_bodies true, got nil")
	}
}

// TestValidatePrivacyHeadersFails verifies that enabling header collection is rejected.
func TestValidatePrivacyHeadersFails(t *testing.T) {
	cfg := defaults()
	cfg.Privacy.CollectHeaders = true

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for collect_headers true, got nil")
	}
}

// TestValidatePrivacyUserIdentifiersFails verifies that enabling user identifier collection
func TestValidatePrivacyUserIdentifiersFails(t *testing.T) {
	cfg := defaults()
	cfg.Privacy.CollectUserIdentifiers = true

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for collect_user_identifiers true, got nil")
	}
}

// TestValidateInvalidRBACMode verifies that an unknown RBAC mode is rejected
func TestValidateInvalidRBACMode(t *testing.T) {
	cfg := defaults()
	cfg.RBAC.Mode = "admin" //Not valid yet

	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for rbac.mode 'admin', got nil")
	}
}

// TestLoadAirgappedYAML verifies that an airgapped config file is parsed correctly
func TestLoadAirgappedYAML(t *testing.T) {
	yaml := `
deployment_mode: airgapped
egress:
  enabled: false
privacy:
  collect_prompt_bodies: false
  collect_response_bodies: false
  collect_headers: false
  collect_user_identifiers: false
rbac:
  mode: read-only
prometheus:
  url: http://prometheus.monitoring.svc.cluster.local:9090
`

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.DeploymentMode != "airgapped" {
		t.Errorf("expected deployment_mode 'airgapped', got %q", cfg.DeploymentMode)
	}

	if cfg.Egress.Enabled {
		t.Error("expected egress.enabled false")
	}
	if cfg.Privacy.CollectPromptBodies {
		t.Error("expected collect_prompt_bodies false")
	}
	if cfg.RBAC.Mode != "read-only" {
		t.Errorf("expected rbac.mode 'read-only', got %q", cfg.RBAC.Mode)
	}
	if cfg.Prometheus.URL != "http://prometheus.monitoring.svc.cluster.local:9090" {
		t.Errorf("unexpected prometheus.url: %q", cfg.Prometheus.URL)
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid airgapped config to pass Validate(), got: %v", err)
	}

}

// TestLoadMissingFileReturnsDefaults verifies that a missing config file
// returns defaults without error
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load("/tmp/does-not-exist-ifa-config.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing config, got: %v", err)
	}
	if cfg.DeploymentMode != "standard" {
		t.Errorf("expected default deployment_mode 'standard', got %q", cfg.DeploymentMode)
	}
}
