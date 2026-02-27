package artifacthandler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// Register registers artifact management tools.
func Register(reg *mcp.Registry, rootDir string) {
	// c4_artifact_save
	reg.Register(mcp.ToolSchema{
		Name:        "c4_artifact_save",
		Description: "Save a file as a versioned artifact with content-hash",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source_path": map[string]any{"type": "string", "description": "Path to source file"},
				"name":        map[string]any{"type": "string", "description": "Artifact name"},
				"description": map[string]any{"type": "string", "description": "Artifact description"},
			},
			"required": []string{"source_path", "name"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleArtifactSave(rootDir, args)
	})

	// c4_artifact_list
	reg.Register(mcp.ToolSchema{
		Name:        "c4_artifact_list",
		Description: "List all saved artifacts",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleArtifactList(rootDir)
	})

	// c4_artifact_get
	reg.Register(mcp.ToolSchema{
		Name:        "c4_artifact_get",
		Description: "Get artifact metadata and content path by name",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Artifact name"},
			},
			"required": []string{"name"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handleArtifactGet(rootDir, args)
	})
}

type artifactSaveArgs struct {
	SourcePath  string `json:"source_path"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func handleArtifactSave(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args artifactSaveArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	sourcePath, err := resolvePath(rootDir, args.SourcePath)
	if err != nil {
		return nil, err
	}

	// Read source file and compute hash
	f, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("opening source: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	data, err := io.ReadAll(io.TeeReader(f, h))
	if err != nil {
		return nil, fmt.Errorf("reading source: %w", err)
	}
	hash := fmt.Sprintf("%x", h.Sum(nil))[:12]

	// Store artifact
	artifactDir := filepath.Join(rootDir, ".c4", "artifacts", args.Name)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("creating artifact dir: %w", err)
	}

	// Copy file with hash in name
	ext := filepath.Ext(args.SourcePath)
	destName := fmt.Sprintf("%s-%s%s", args.Name, hash, ext)
	destPath := filepath.Join(artifactDir, destName)

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return nil, fmt.Errorf("writing artifact: %w", err)
	}

	// Save metadata
	meta := map[string]any{
		"name":        args.Name,
		"hash":        hash,
		"source":      args.SourcePath,
		"description": args.Description,
		"size":        len(data),
		"created_at":  time.Now().Format(time.RFC3339),
		"path":        destPath,
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	metaPath := filepath.Join(artifactDir, "metadata.json")
	if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing metadata: %w", err)
	}

	return map[string]any{
		"success": true,
		"name":    args.Name,
		"hash":    hash,
		"path":    destPath,
		"size":    len(data),
	}, nil
}

func handleArtifactList(rootDir string) (any, error) {
	artifactsDir := filepath.Join(rootDir, ".c4", "artifacts")
	entries, err := os.ReadDir(artifactsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"artifacts": []any{}, "count": 0}, nil
		}
		return nil, fmt.Errorf("reading artifacts dir: %w", err)
	}

	var artifacts []map[string]any
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(artifactsDir, e.Name(), "metadata.json")
		metaData, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal(metaData, &meta); err != nil {
			continue
		}
		artifacts = append(artifacts, meta)
	}

	return map[string]any{
		"artifacts": artifacts,
		"count":     len(artifacts),
	}, nil
}

type artifactGetArgs struct {
	Name string `json:"name"`
}

func handleArtifactGet(rootDir string, rawArgs json.RawMessage) (any, error) {
	var args artifactGetArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("parsing arguments: %w", err)
	}

	metaPath := filepath.Join(rootDir, ".c4", "artifacts", args.Name, "metadata.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"error": fmt.Sprintf("artifact '%s' not found", args.Name)}, nil
		}
		return nil, fmt.Errorf("reading metadata: %w", err)
	}

	var meta map[string]any
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata: %w", err)
	}

	return meta, nil
}

// resolvePath resolves a path relative to rootDir, preventing directory traversal.
func resolvePath(rootDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		cleaned := filepath.Clean(path)
		if !strings.HasPrefix(cleaned, filepath.Clean(rootDir)) {
			return "", fmt.Errorf("absolute path escapes project root: %s", path)
		}
		return cleaned, nil
	}
	resolved := filepath.Join(rootDir, path)
	resolved = filepath.Clean(resolved)
	if !strings.HasPrefix(resolved, filepath.Clean(rootDir)) {
		return "", fmt.Errorf("path escapes project root: %s", path)
	}
	return resolved, nil
}
