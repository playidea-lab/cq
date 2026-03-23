//go:build integration

package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFixCloudState(t *testing.T) {
	home, _ := os.UserHomeDir()
	projectDir := filepath.Join(home, "git/cq")

	cfgData, err := os.ReadFile(filepath.Join(projectDir, ".c4/config.yaml"))
	if err != nil {
		t.Skipf("config not found: %v", err)
	}
	var cfg struct {
		Cloud struct {
			URL     string `yaml:"url"`
			AnonKey string `yaml:"anon_key"`
		} `yaml:"cloud"`
	}
	yaml.Unmarshal(cfgData, &cfg)
	if cfg.Cloud.URL == "" {
		t.Skip("no cloud URL")
	}

	sessData, err := os.ReadFile(filepath.Join(home, ".c4/session.json"))
	if err != nil {
		t.Skipf("session not found: %v", err)
	}
	var sess struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(sessData, &sess)
	if sess.AccessToken == "" {
		t.Skip("no access token")
	}

	// Load cloud project ID from config
	var fullCfg struct {
		Cloud struct {
			ProjectID       string `yaml:"project_id"`
			ActiveProjectID string `yaml:"active_project_id"`
		} `yaml:"cloud"`
	}
	yaml.Unmarshal(cfgData, &fullCfg)
	projectID := fullCfg.Cloud.ActiveProjectID
	if projectID == "" {
		projectID = fullCfg.Cloud.ProjectID
	}
	t.Logf("project_id: %s", projectID)

	supabaseURL := cfg.Cloud.URL + "/rest/v1"
	cs := NewCloudStore(supabaseURL, cfg.Cloud.AnonKey,
		NewStaticTokenProvider(sess.AccessToken), projectID)

	// Read current state
	status, err := cs.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	t.Logf("Before: state=%s tasks=%d", status.State, status.TotalTasks)

	// Call Start (new code: always → EXECUTE)
	err = cs.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify
	status2, err := cs.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus after: %v", err)
	}
	t.Logf("After: state=%s tasks=%d", status2.State, status2.TotalTasks)

	if status2.State != "EXECUTE" {
		t.Errorf("expected EXECUTE, got %s", status2.State)
	}
}
