package handlers

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/mcp/apps"
	_ "modernc.org/sqlite"
)

func newDashboardTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	return store
}

func TestDashboard_TextFormat(t *testing.T) {
	store := newDashboardTestStore(t)
	deps := &DashboardDeps{}

	result, err := handleDashboard(store, deps, "text")
	if err != nil {
		t.Fatalf("handleDashboard text: %v", err)
	}

	data, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Must contain required top-level keys
	for _, key := range []string{"status", "memory_count", "nodes", "jobs_total", "jobs_running"} {
		if _, exists := data[key]; !exists {
			t.Errorf("missing key %q in text response", key)
		}
	}

	// text format must not include _meta
	if _, hasMeta := data["_meta"]; hasMeta {
		t.Error("text format must not include _meta")
	}
}

func TestDashboard_WidgetFormat(t *testing.T) {
	store := newDashboardTestStore(t)
	deps := &DashboardDeps{}

	result, err := handleDashboard(store, deps, "widget")
	if err != nil {
		t.Fatalf("handleDashboard widget: %v", err)
	}

	outer, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// widget format: {data: {...}, _meta: {...}}
	dataRaw, hasData := outer["data"]
	if !hasData {
		t.Fatal("widget response must have 'data' key")
	}
	data, ok := dataRaw.(map[string]any)
	if !ok {
		t.Fatalf("data must be map[string]any, got %T", dataRaw)
	}

	metaRaw, hasMeta := outer["_meta"]
	if !hasMeta {
		t.Fatal("widget response must have '_meta' key")
	}

	// Verify _meta.ui.resourceUri
	meta, ok := metaRaw.(map[string]any)
	if !ok {
		t.Fatalf("_meta must be map[string]any, got %T", metaRaw)
	}
	uiRaw, hasUI := meta["ui"]
	if !hasUI {
		t.Fatal("_meta must have 'ui' key")
	}
	ui, ok := uiRaw.(map[string]any)
	if !ok {
		t.Fatalf("_meta.ui must be map[string]any, got %T", uiRaw)
	}
	uri, _ := ui["resourceUri"].(string)
	if uri != dashboardResourceURI {
		t.Errorf("resourceUri = %q, want %q", uri, dashboardResourceURI)
	}

	// data must have required fields
	for _, key := range []string{"status", "memory_count", "nodes", "jobs_total", "jobs_running"} {
		if _, exists := data[key]; !exists {
			t.Errorf("missing key %q in widget data", key)
		}
	}
}

func TestDashboard_ResourceStoreRegistration(t *testing.T) {
	rs := apps.NewResourceStore()
	html := "<html>test dashboard</html>"
	store := newDashboardTestStore(t)
	deps := &DashboardDeps{
		ResourceStore: rs,
		DashboardHTML: html,
	}

	reg := mcp.NewRegistry()
	RegisterDashboardHandler(reg, store, deps)

	// Widget HTML should be registered in the resource store
	content, mime, err := rs.HandleResourcesRead(dashboardResourceURI)
	if err != nil {
		t.Fatalf("HandleResourcesRead: %v", err)
	}
	if content != html {
		t.Errorf("content = %q, want %q", content, html)
	}
	if mime != "text/html" {
		t.Errorf("mime = %q, want text/html", mime)
	}
}

func TestDashboard_MCPToolRegistered(t *testing.T) {
	reg := mcp.NewRegistry()
	store := newDashboardTestStore(t)

	RegisterDashboardHandler(reg, store, nil)

	// Tool must be discoverable
	tools := reg.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "c4_dashboard" {
			found = true
			break
		}
	}
	if !found {
		t.Error("c4_dashboard tool not found in registry")
	}
}

func TestDashboard_DefaultFormatIsText(t *testing.T) {
	store := newDashboardTestStore(t)
	deps := &DashboardDeps{}

	// Empty format string → defaults to text (no _meta)
	result, err := handleDashboard(store, deps, "")
	if err != nil {
		t.Fatalf("handleDashboard default: %v", err)
	}

	data, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if _, hasMeta := data["_meta"]; hasMeta {
		t.Error("default format must not include _meta")
	}
}

func TestDashboard_ViaRegistryCall(t *testing.T) {
	store := newDashboardTestStore(t)

	reg := mcp.NewRegistry()
	RegisterDashboardHandler(reg, store, nil)

	// Call via registry with widget format
	raw, err := json.Marshal(map[string]string{"format": "widget"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	result, err := reg.Call("c4_dashboard", raw)
	if err != nil {
		t.Fatalf("registry call: %v", err)
	}

	outer, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if _, hasData := outer["data"]; !hasData {
		t.Error("widget response must have 'data' key")
	}
	if _, hasMeta := outer["_meta"]; !hasMeta {
		t.Error("widget response must have '_meta' key")
	}
}

func TestDashboard_NilStore(t *testing.T) {
	deps := &DashboardDeps{}
	// nil store should not panic — returns safe defaults
	result, err := handleDashboard(nil, deps, "text")
	if err != nil {
		t.Fatalf("handleDashboard nil store: %v", err)
	}
	data, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if data["status"] != "online" {
		t.Errorf("nil store status = %v, want 'online'", data["status"])
	}
}
