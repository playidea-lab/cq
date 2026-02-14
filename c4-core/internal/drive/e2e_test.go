package drive

import (
	"os"
	"testing"
)

// TestE2EDrive runs end-to-end tests against a real Supabase instance.
// Skipped unless SUPABASE_URL, SUPABASE_KEY, and ACCESS_TOKEN are set.
func TestE2EDrive(t *testing.T) {
	url := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_KEY")
	token := os.Getenv("ACCESS_TOKEN")
	if url == "" || key == "" || token == "" {
		t.Skip("Skipping E2E: SUPABASE_URL, SUPABASE_KEY, ACCESS_TOKEN required")
	}

	projectID := os.Getenv("C4_CLOUD_PROJECT_UUID")
	if projectID == "" {
		t.Skip("Skipping E2E: C4_CLOUD_PROJECT_UUID required")
	}
	client := NewClient(url, key, token, projectID)

	// Create test file
	tmpFile := t.TempDir() + "/e2e_test.txt"
	if err := os.WriteFile(tmpFile, []byte("C4 Drive E2E test content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Upload
	t.Run("Upload", func(t *testing.T) {
		info, err := client.Upload(tmpFile, "/e2e-test/hello.txt", nil)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		if info.Name != "hello.txt" {
			t.Errorf("Name = %q, want hello.txt", info.Name)
		}
		if info.SizeBytes != 25 {
			t.Errorf("SizeBytes = %d, want 25", info.SizeBytes)
		}
		t.Logf("Upload OK: path=%s hash=%s", info.Path, info.ContentHash[:16])
	})

	// 2. List
	t.Run("List", func(t *testing.T) {
		files, err := client.List("/e2e-test/")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(files) == 0 {
			t.Fatal("List returned 0 files, expected at least 1")
		}
		found := false
		for _, f := range files {
			if f.Path == "/e2e-test/hello.txt" || f.Path == "e2e-test/hello.txt" {
				found = true
				t.Logf("List OK: found %s", f.Path)
			}
		}
		if !found {
			t.Errorf("did not find e2e-test/hello.txt in list: %+v", files)
		}
	})

	// 3. Info
	t.Run("Info", func(t *testing.T) {
		info, err := client.Info("/e2e-test/hello.txt")
		if err != nil {
			t.Fatalf("Info failed: %v", err)
		}
		if info.Name != "hello.txt" {
			t.Errorf("Name = %q, want hello.txt", info.Name)
		}
		t.Logf("Info OK: name=%s type=%s size=%d", info.Name, info.ContentType, info.SizeBytes)
	})

	// 4. Download
	t.Run("Download", func(t *testing.T) {
		destFile := t.TempDir() + "/downloaded.txt"
		if err := client.Download("/e2e-test/hello.txt", destFile); err != nil {
			t.Fatalf("Download failed: %v", err)
		}
		data, err := os.ReadFile(destFile)
		if err != nil {
			t.Fatalf("read downloaded file: %v", err)
		}
		if string(data) != "C4 Drive E2E test content" {
			t.Errorf("content = %q, want 'C4 Drive E2E test content'", string(data))
		}
		t.Logf("Download OK: %d bytes", len(data))
	})

	// 5. Delete
	t.Run("Delete", func(t *testing.T) {
		if err := client.Delete("/e2e-test/hello.txt"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		t.Log("Delete OK")
	})

	// 6. Verify deleted
	t.Run("VerifyDeleted", func(t *testing.T) {
		_, err := client.Info("/e2e-test/hello.txt")
		if err == nil {
			t.Error("Info should fail for deleted file")
		} else {
			t.Logf("Verify OK: %v", err)
		}
	})
}
