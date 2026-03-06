package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/drive"
	"github.com/spf13/cobra"
)

// driveCmd is the top-level `cq drive` command.
var driveCmd = &cobra.Command{
	Use:   "drive",
	Short: "Manage CQ Drive file storage",
}

// datasetCmd is the `cq drive dataset` command group.
var datasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Manage datasets in CQ Drive",
	Long: `Manage datasets stored in CQ Drive (Supabase Storage).

Subcommands:
  upload <path> --as <name>  - Upload a dataset directory
  list [<name>]              - List datasets or versions
  pull <name>                - Download a dataset`,
}

var datasetUploadCmd = &cobra.Command{
	Use:   "upload <path>",
	Short: "Upload a dataset directory to CQ Drive",
	Args:  cobra.ExactArgs(1),
	RunE:  runDatasetUpload,
}

var datasetListCmd = &cobra.Command{
	Use:   "list [<name>]",
	Short: "List datasets or versions of a dataset",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDatasetList,
}

var datasetPullCmd = &cobra.Command{
	Use:   "pull <name>",
	Short: "Download a dataset from CQ Drive",
	Args:  cobra.ExactArgs(1),
	RunE:  runDatasetPull,
}

func init() {
	datasetUploadCmd.Flags().String("as", "", "Dataset name (required)")
	datasetUploadCmd.MarkFlagRequired("as") //nolint:errcheck
	datasetUploadCmd.Flags().String("ignore", "", "Path to ignore file (e.g. .cqdriveignore)")

	datasetPullCmd.Flags().String("dest", ".", "Destination directory")
	datasetPullCmd.Flags().String("version", "", "Version hash to pull (default: latest)")

	datasetCmd.AddCommand(datasetUploadCmd)
	datasetCmd.AddCommand(datasetListCmd)
	datasetCmd.AddCommand(datasetPullCmd)
	driveCmd.AddCommand(datasetCmd)
	rootCmd.AddCommand(driveCmd)
}

// validateDatasetName rejects names with path separators or dot-dot.
func validateDatasetName(name string) error {
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid dataset name %q: must not contain path separators or '..'", name)
	}
	return nil
}

// newDriveClient creates a Drive client from config/env/session.
func newDriveClient() (*drive.Client, error) {
	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return nil, errors.New("no Supabase URL configured (set C4_CLOUD_URL or run 'cq auth login')")
	}
	anonKey := readCloudAnonKey(projectDir)
	if anonKey == "" {
		return nil, errors.New("no Supabase anon key configured (set C4_CLOUD_ANON_KEY or run 'cq auth login')")
	}

	authClient := cloud.NewAuthClient(supabaseURL, anonKey)
	session, err := authClient.GetSession()
	if err != nil {
		return nil, fmt.Errorf("loading session: %w", err)
	}
	if session == nil {
		return nil, errors.New("not authenticated: run 'cq auth login'")
	}
	if time.Now().Unix() >= session.ExpiresAt {
		return nil, errors.New("session expired: run 'cq auth login'")
	}

	projectID := getActiveProjectID(projectDir)
	if projectID == "" {
		return nil, errors.New("no active project: run 'cq project use <id>'")
	}

	tp := &staticToken{token: session.AccessToken}
	return drive.NewClient(supabaseURL, anonKey, tp, projectID), nil
}

// staticToken implements drive.tokenProvider for a fixed access token.
type staticToken struct{ token string }

func (s *staticToken) Token() string { return s.token }

// datasetDrivePath returns the drive path for a dataset file.
// Pattern: /datasets/<name>/<versionHash>/<relPath>
func datasetDrivePath(name, versionHash, relPath string) string {
	return "/datasets/" + name + "/" + versionHash + "/" + relPath
}

// computeDirHash computes a stable SHA256 hash for a set of WalkEntries.
// The hash is derived from each file's relative path and its content hash.
func computeDirHash(entries []drive.WalkEntry) (string, error) {
	h := sha256.New()
	for _, e := range entries {
		fh, err := fileHash(e.Path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\t%s\n", e.RelPath, fh)
	}
	return hex.EncodeToString(h.Sum(nil))[:8], nil
}

// fileHash computes the SHA256 hex of a local file.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func runDatasetUpload(cmd *cobra.Command, args []string) error {
	srcPath := args[0]
	name, _ := cmd.Flags().GetString("as")
	ignoreFile, _ := cmd.Flags().GetString("ignore")

	if err := validateDatasetName(name); err != nil {
		return err
	}

	client, err := newDriveClient()
	if err != nil {
		return err
	}

	entries, err := drive.WalkDir(srcPath, ignoreFile)
	if err != nil {
		return fmt.Errorf("scanning directory: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("no files found in %s", srcPath)
	}

	versionHash, err := computeDirHash(entries)
	if err != nil {
		return fmt.Errorf("computing version hash: %w", err)
	}

	// Check if this version already exists by probing the first file.
	firstDrivePath := datasetDrivePath(name, versionHash, entries[0].RelPath)
	if _, err := client.Info(firstDrivePath); err == nil {
		fmt.Printf("dataset '%s' v=%s unchanged\n", name, versionHash)
		return nil
	}

	var uploaded, skipped int
	var totalBytes int64

	for _, e := range entries {
		drivePath := datasetDrivePath(name, versionHash, e.RelPath)

		// Check if remote already has this file (same content hash).
		localHash, hashErr := fileHash(e.Path)
		if hashErr == nil {
			if info, err := client.Info(drivePath); err == nil && info.ContentHash == "sha256:"+localHash {
				skipped++
				totalBytes += e.Size
				continue
			}
		}

		if _, err := client.Upload(e.Path, drivePath, nil); err != nil {
			return fmt.Errorf("upload %s: %w", e.RelPath, err)
		}
		uploaded++
		totalBytes += e.Size
	}

	fmt.Printf("dataset '%s' v=%s uploaded (%d files, %d skipped, %s)\n",
		name, versionHash, len(entries), skipped, formatBytes(totalBytes))
	return nil
}

// datasetSummary holds aggregated info per dataset name for `list`.
type datasetSummary struct {
	name     string
	latest   string
	files    int
	size     int64
	versions int
}

func runDatasetList(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		if err := validateDatasetName(args[0]); err != nil {
			return err
		}
	}

	client, err := newDriveClient()
	if err != nil {
		return err
	}

	folder := "/datasets"
	if len(args) == 1 {
		folder = "/datasets/" + args[0]
	}

	files, err := client.List(folder)
	if err != nil {
		return fmt.Errorf("listing datasets: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No datasets found.")
		return nil
	}

	// Group by dataset name → version hash → count files/size.
	// Drive paths: /datasets/<name>/<version>/<relpath>
	type versionKey struct{ name, version string }
	versionFiles := map[versionKey]int{}
	versionSize := map[versionKey]int64{}
	nameVersions := map[string]map[string]struct{}{}
	nameLatest := map[string]string{}

	for _, f := range files {
		// Path format: /datasets/<name>/<version>/...
		parts := strings.SplitN(strings.TrimPrefix(f.Path, "/datasets/"), "/", 3)
		if len(parts) < 2 {
			continue
		}
		dsName, ver := parts[0], parts[1]
		if dsName == "" || ver == "" {
			continue
		}
		k := versionKey{dsName, ver}
		if !f.IsFolder {
			versionFiles[k]++
			versionSize[k] += f.SizeBytes
		}
		if nameVersions[dsName] == nil {
			nameVersions[dsName] = map[string]struct{}{}
		}
		nameVersions[dsName][ver] = struct{}{}
		// Use alphabetically last version as "latest" approximation.
		if ver > nameLatest[dsName] {
			nameLatest[dsName] = ver
		}
	}

	// Build ordered list of summaries.
	seen := map[string]bool{}
	var summaries []datasetSummary
	for _, f := range files {
		parts := strings.SplitN(strings.TrimPrefix(f.Path, "/datasets/"), "/", 3)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		dsName := parts[0]
		if seen[dsName] {
			continue
		}
		seen[dsName] = true
		latest := nameLatest[dsName]
		k := versionKey{dsName, latest}
		summaries = append(summaries, datasetSummary{
			name:     dsName,
			latest:   latest,
			files:    versionFiles[k],
			size:     versionSize[k],
			versions: len(nameVersions[dsName]),
		})
	}

	fmt.Printf("%-30s %-10s %6s %12s %8s\n", "NAME", "LATEST", "FILES", "SIZE", "VERSIONS")
	fmt.Println(strings.Repeat("-", 72))
	for _, s := range summaries {
		fmt.Printf("%-30s %-10s %6d %12s %8d\n",
			s.name, s.latest, s.files, formatBytes(s.size), s.versions)
	}
	return nil
}

func runDatasetPull(cmd *cobra.Command, args []string) error {
	name := args[0]
	destDir, _ := cmd.Flags().GetString("dest")
	version, _ := cmd.Flags().GetString("version")

	if err := validateDatasetName(name); err != nil {
		return err
	}

	client, err := newDriveClient()
	if err != nil {
		return err
	}

	// List files for this dataset to find the target version.
	files, err := client.List("/datasets/" + name)
	if err != nil {
		return fmt.Errorf("listing dataset %q: %w", name, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("dataset %q not found", name)
	}

	// Determine target version hash.
	if version == "" {
		// Find latest (alphabetically last).
		for _, f := range files {
			parts := strings.SplitN(strings.TrimPrefix(f.Path, "/datasets/"+name+"/"), "/", 2)
			if len(parts) >= 1 && parts[0] > version {
				version = parts[0]
			}
		}
	}
	if version == "" {
		return fmt.Errorf("no versions found for dataset %q", name)
	}

	// Collect files for the target version.
	var targets []string
	for _, f := range files {
		if !f.IsFolder && strings.HasPrefix(f.Path, "/datasets/"+name+"/"+version+"/") {
			targets = append(targets, f.Path)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("no files found for dataset %q version %s", name, version)
	}

	var downloaded, skipped int
	prefix := "/datasets/" + name + "/" + version + "/"

	for _, drivePath := range targets {
		relPath := strings.TrimPrefix(drivePath, prefix)
		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))

		// Skip if local file already exists with matching content.
		if _, err := os.Stat(destPath); err == nil {
			if info, err := client.Info(drivePath); err == nil {
				if lh, err := fileHash(destPath); err == nil && "sha256:"+lh == info.ContentHash {
					skipped++
					continue
				}
			}
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", relPath, err)
		}
		if err := client.Download(drivePath, destPath); err != nil {
			return fmt.Errorf("download %s: %w", relPath, err)
		}
		downloaded++
	}

	fmt.Printf("'%s@%s' -> %s/ (%d downloaded, %d skipped)\n",
		name, version, destDir, downloaded, skipped)
	return nil
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
