package apps_test

import (
	"testing"

	"github.com/changmin/c4-core/internal/mcp/apps"
)

func TestResourceStore_RegisterAndGet(t *testing.T) {
	s := apps.NewResourceStore()

	s.Register("ui://c4/task-card", "<html>task</html>")

	r, ok := s.Get("ui://c4/task-card")
	if !ok {
		t.Fatal("expected resource to be found")
	}
	if r.Uri != "ui://c4/task-card" {
		t.Errorf("unexpected Uri: %q", r.Uri)
	}
	if r.Content != "<html>task</html>" {
		t.Errorf("unexpected Content: %q", r.Content)
	}
	if r.ContentType != "text/html" {
		t.Errorf("unexpected ContentType: %q", r.ContentType)
	}
}

func TestResourceStore_Get_NotFound(t *testing.T) {
	s := apps.NewResourceStore()
	_, ok := s.Get("ui://c4/nonexistent")
	if ok {
		t.Fatal("expected resource not to be found")
	}
}

func TestResourceStore_HandleResourcesRead_Success(t *testing.T) {
	s := apps.NewResourceStore()
	s.Register("ui://c4/widget", "<div>hello</div>")

	content, mime, err := s.HandleResourcesRead("ui://c4/widget")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "<div>hello</div>" {
		t.Errorf("unexpected content: %q", content)
	}
	if mime != "text/html" {
		t.Errorf("unexpected mime: %q", mime)
	}
}

func TestResourceStore_HandleResourcesRead_NotFound(t *testing.T) {
	s := apps.NewResourceStore()
	_, _, err := s.HandleResourcesRead("ui://c4/missing")
	if err == nil {
		t.Fatal("expected error for missing resource")
	}
}

func TestResourceStore_HandleResourcesRead_BadScheme(t *testing.T) {
	s := apps.NewResourceStore()
	_, _, err := s.HandleResourcesRead("https://example.com/page")
	if err == nil {
		t.Fatal("expected error for non-ui:// URI")
	}
}

func TestUIResource_Fields(t *testing.T) {
	r := apps.UIResource{
		Uri:         "ui://c4/test",
		Content:     "<p>hi</p>",
		ContentType: "text/html",
	}
	if r.Uri == "" || r.Content == "" || r.ContentType == "" {
		t.Error("UIResource fields should not be empty")
	}
}
