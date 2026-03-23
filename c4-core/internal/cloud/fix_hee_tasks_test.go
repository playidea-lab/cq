//go:build integration

package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFixHEETasksExecutionMode(t *testing.T) {
	home, _ := os.UserHomeDir()
	projectDir := filepath.Join(home, "git/cq")

	cfgData, err := os.ReadFile(filepath.Join(projectDir, ".c4/config.yaml"))
	if err != nil {
		t.Skipf("config not found: %v", err)
	}
	var cfg struct {
		Cloud struct {
			URL             string `yaml:"url"`
			AnonKey         string `yaml:"anon_key"`
			ActiveProjectID string `yaml:"active_project_id"`
		} `yaml:"cloud"`
	}
	yaml.Unmarshal(cfgData, &cfg)

	sessData, err := os.ReadFile(filepath.Join(home, ".c4/session.json"))
	if err != nil {
		t.Skipf("session not found: %v", err)
	}
	var sess struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(sessData, &sess)

	cs := NewCloudStore(cfg.Cloud.URL+"/rest/v1", cfg.Cloud.AnonKey,
		NewStaticTokenProvider(sess.AccessToken), cfg.Cloud.ActiveProjectID)

	// Patch tasks to direct mode
	taskIDs := []string{"T-RTT-001-0", "T-RTT-002-0", "T-RTT-003-0"}
	for _, tid := range taskIDs {
		err := cs.patch("c4_tasks", "task_id=eq."+tid, map[string]any{
			"execution_mode": "direct",
			"status":         "pending",
		})
		if err != nil {
			t.Logf("patch %s: %v (may not exist in cloud)", tid, err)
		} else {
			t.Logf("✅ %s → direct/pending", tid)
		}
	}
}
