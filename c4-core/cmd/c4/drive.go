package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/drive"
	"github.com/spf13/cobra"
)

// cqdataShortLen is the number of version hash characters shown in the .cqdata hint.
const cqdataShortLen = 8

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
	datasetUploadCmd.Flags().Bool("update-cqdata", true, "Update .cqdata with name+version after upload")

	datasetPullCmd.Flags().String("dest", ".", "Destination directory")
	datasetPullCmd.Flags().String("version", "", "Version hash to pull (default: latest)")

	datasetCmd.AddCommand(datasetUploadCmd)
	datasetCmd.AddCommand(datasetListCmd)
	datasetCmd.AddCommand(datasetPullCmd)
	driveCmd.AddCommand(datasetCmd)
	rootCmd.AddCommand(driveCmd)
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

// newDatasetClient creates a DatasetClient from config/env/session.
func newDatasetClient() (*drive.DatasetClient, error) {
	c, err := newDriveClient()
	if err != nil {
		return nil, err
	}
	return drive.NewDatasetClient(c), nil
}

func runDatasetUpload(cmd *cobra.Command, args []string) error {
	srcPath := args[0]
	name, _ := cmd.Flags().GetString("as")
	ignoreFile, _ := cmd.Flags().GetString("ignore")
	updateCQData, _ := cmd.Flags().GetBool("update-cqdata")

	dc, err := newDatasetClient()
	if err != nil {
		return err
	}

	result, err := dc.Upload(cmd.Context(), srcPath, name, ignoreFile)
	if err != nil {
		return err
	}
	if !result.Changed {
		fmt.Printf("dataset '%s' v=%s unchanged\n", result.Name, result.VersionHash)
		return nil
	}
	fmt.Printf("dataset '%s' v=%s uploaded (%d files, %d skipped, %s)\n",
		result.Name, result.VersionHash, result.FilesUploaded, result.FilesSkipped,
		formatBytes(result.TotalSizeBytes))
	if updateCQData {
		if err := drive.ApplyCQData(projectDir, result.Name, result.VersionHash); err != nil {
			return fmt.Errorf("update .cqdata: %w", err)
		}
		short := result.VersionHash
		if len(short) > cqdataShortLen {
			short = short[:cqdataShortLen]
		}
		fmt.Printf("Updated .cqdata: %s → %s. Run: git add .cqdata\n", result.Name, short)
	}
	return nil
}

func runDatasetList(cmd *cobra.Command, args []string) error {
	nameFilter := ""
	if len(args) == 1 {
		nameFilter = args[0]
	}

	dc, err := newDatasetClient()
	if err != nil {
		return err
	}

	versions, err := dc.List(cmd.Context(), nameFilter)
	if err != nil {
		return fmt.Errorf("listing datasets: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No datasets found.")
		return nil
	}

	// Group versions by name; List returns newest-first from DB.
	type summary struct {
		latest   string
		files    int
		size     int64
		versions int
	}
	byName := map[string]*summary{}
	var order []string
	for _, v := range versions {
		s, ok := byName[v.Name]
		if !ok {
			s = &summary{}
			byName[v.Name] = s
			order = append(order, v.Name)
		}
		s.versions++
		if s.latest == "" {
			// First entry is the newest (DB ordered by created_at DESC).
			s.latest = v.VersionHash
			s.files = v.FileCount
			s.size = v.TotalSizeBytes
		}
	}

	fmt.Printf("%-30s %-16s %6s %12s %8s\n", "NAME", "LATEST", "FILES", "SIZE", "VERSIONS")
	fmt.Println(strings.Repeat("-", 78))
	for _, n := range order {
		s := byName[n]
		latest := s.latest
		if len(latest) > 16 {
			latest = latest[:16]
		}
		fmt.Printf("%-30s %-16s %6d %12s %8d\n",
			n, latest, s.files, formatBytes(s.size), s.versions)
	}
	return nil
}

func runDatasetPull(cmd *cobra.Command, args []string) error {
	name := args[0]
	destDir, _ := cmd.Flags().GetString("dest")
	version, _ := cmd.Flags().GetString("version")

	dc, err := newDatasetClient()
	if err != nil {
		return err
	}

	result, err := dc.Pull(cmd.Context(), name, destDir, version)
	if err != nil {
		return err
	}
	fmt.Printf("'%s@%s' -> %s/ (%d downloaded, %d skipped)\n",
		result.Name, result.VersionHash, result.Dest, result.FilesDownloaded, result.FilesSkipped)
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
