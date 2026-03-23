//go:build integration
package cloud
import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"gopkg.in/yaml.v3"
)
func TestFixRTTTasks(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfgData, _ := os.ReadFile(filepath.Join(home, "git/cq/.c4/config.yaml"))
	var cfg struct{ Cloud struct{ URL, AnonKey, ActiveProjectID string `yaml:"url,anon_key,active_project_id"` } `yaml:"cloud"` }
	yaml.Unmarshal(cfgData, &cfg)
	sessData, _ := os.ReadFile(filepath.Join(home, ".c4/session.json"))
	var sess struct{ AccessToken string `json:"access_token"` }
	json.Unmarshal(sessData, &sess)
	cs := NewCloudStore(cfg.Cloud.URL+"/rest/v1", cfg.Cloud.AnonKey, NewStaticTokenProvider(sess.AccessToken), cfg.Cloud.ActiveProjectID)
	for _, tid := range []string{"T-RTT-001-0", "T-RTT-002-0", "T-RTT-003-0"} {
		err := cs.patch("c4_tasks", "task_id=eq."+tid, map[string]any{"execution_mode": "direct", "status": "pending"})
		if err != nil { t.Logf("%s: %v", tid, err) } else { t.Logf("✅ %s", tid) }
	}
}
