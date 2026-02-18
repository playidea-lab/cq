package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/changmin/c4-core/internal/eventbus"
	pb "github.com/changmin/c4-core/internal/eventbus/pb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
)

var (
	eventbusSocket  string
	eventbusDataDir string
)

var eventbusCmd = &cobra.Command{
	Use:   "eventbus",
	Short: "C3 EventBus daemon and management commands",
	Long: `C3 EventBus — event-driven pipelines between C4 components.

Start the gRPC daemon, manage rules, view logs, and monitor events.

Example:
  cq eventbus              # Start daemon
  cq eventbus status       # Show daemon stats
  cq eventbus logs         # View dispatch logs
  cq eventbus rules list   # List routing rules
  cq eventbus monitor      # Live event stream`,
	RunE: runEventbus,
}

var eventbusStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running eventbus daemon",
	RunE:  runEventbusStop,
}

// --- logs ---

var eventbusLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View dispatch logs",
	RunE:  runEventbusLogs,
}

var logsType string
var logsLimit int
var logsSince string

// --- rules ---

var eventbusRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Manage event routing rules",
}

var eventbusRulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all routing rules",
	RunE:  runRulesList,
}

var eventbusRulesAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new routing rule",
	RunE:  runRulesAdd,
}

var ruleAddName, ruleAddPattern, ruleAddAction, ruleAddConfig, ruleAddFilter string
var ruleAddEnabled bool
var ruleAddPriority int

var eventbusRulesRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a routing rule",
	RunE:  runRulesRemove,
}

var ruleRemoveName string

var eventbusRulesToggleCmd = &cobra.Command{
	Use:   "toggle",
	Short: "Enable or disable a rule",
	RunE:  runRulesToggle,
}

var ruleToggleName string
var ruleToggleEnabled bool

var eventbusRulesImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import rules from YAML file",
	RunE:  runRulesImport,
}

var ruleImportFile string

// --- monitor ---

var eventbusMonitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Live event stream (subscribe)",
	RunE:  runMonitor,
}

var monitorPattern string

// --- status ---

var eventbusStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status and statistics",
	RunE:  runEventbusStatus,
}

// --- dlq ---

var eventbusDLQCmd = &cobra.Command{
	Use:   "dlq",
	Short: "View and manage the Dead Letter Queue",
	RunE:  runDLQ,
}

var dlqLimit int
var dlqRetryID int64
var dlqPurge bool
var dlqPurgeAge string

// --- replay ---

var eventbusReplayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay stored events",
	RunE:  runReplay,
}

var replayType string
var replaySince string
var replayLimit int
var replayDryRun bool

func init() {
	eventbusCmd.Flags().StringVar(&eventbusSocket, "socket", "", "Unix socket path (default: ~/.c4/eventbus/c3.sock)")
	eventbusCmd.Flags().StringVar(&eventbusDataDir, "data-dir", "", "data directory (default: ~/.c4/eventbus)")

	// logs
	eventbusLogsCmd.Flags().StringVar(&logsType, "type", "", "filter by event type")
	eventbusLogsCmd.Flags().IntVar(&logsLimit, "limit", 50, "max entries")
	eventbusLogsCmd.Flags().StringVar(&logsSince, "since", "", "only logs after this time (RFC3339)")

	// rules add
	eventbusRulesAddCmd.Flags().StringVar(&ruleAddName, "name", "", "rule name (required)")
	eventbusRulesAddCmd.Flags().StringVar(&ruleAddPattern, "pattern", "", "event pattern (required)")
	eventbusRulesAddCmd.Flags().StringVar(&ruleAddAction, "action", "", "action type (required)")
	eventbusRulesAddCmd.Flags().StringVar(&ruleAddConfig, "config", "{}", "action config JSON")
	eventbusRulesAddCmd.Flags().StringVar(&ruleAddFilter, "filter", "", "filter JSON")
	eventbusRulesAddCmd.Flags().BoolVar(&ruleAddEnabled, "enabled", true, "enable rule")
	eventbusRulesAddCmd.Flags().IntVar(&ruleAddPriority, "priority", 0, "rule priority")

	// rules remove
	eventbusRulesRemoveCmd.Flags().StringVar(&ruleRemoveName, "name", "", "rule name to remove")

	// rules toggle
	eventbusRulesToggleCmd.Flags().StringVar(&ruleToggleName, "name", "", "rule name")
	eventbusRulesToggleCmd.Flags().BoolVar(&ruleToggleEnabled, "enabled", true, "set enabled state")

	// rules import
	eventbusRulesImportCmd.Flags().StringVar(&ruleImportFile, "file", "", "YAML rules file path")

	// monitor
	eventbusMonitorCmd.Flags().StringVar(&monitorPattern, "pattern", "*", "event pattern to subscribe")

	// dlq
	eventbusDLQCmd.Flags().IntVar(&dlqLimit, "limit", 50, "max entries to show")
	eventbusDLQCmd.Flags().Int64Var(&dlqRetryID, "retry", 0, "retry a specific DLQ entry by ID")
	eventbusDLQCmd.Flags().BoolVar(&dlqPurge, "purge", false, "purge old DLQ entries")
	eventbusDLQCmd.Flags().StringVar(&dlqPurgeAge, "age", "7d", "age threshold for purge (e.g. 7d, 24h)")

	// replay
	eventbusReplayCmd.Flags().StringVar(&replayType, "type", "", "filter by event type")
	eventbusReplayCmd.Flags().StringVar(&replaySince, "since", "", "only events after (RFC3339)")
	eventbusReplayCmd.Flags().IntVar(&replayLimit, "limit", 100, "max events")
	eventbusReplayCmd.Flags().BoolVar(&replayDryRun, "dry-run", true, "stream only, don't re-dispatch")

	// Build command tree
	eventbusRulesCmd.AddCommand(eventbusRulesListCmd)
	eventbusRulesCmd.AddCommand(eventbusRulesAddCmd)
	eventbusRulesCmd.AddCommand(eventbusRulesRemoveCmd)
	eventbusRulesCmd.AddCommand(eventbusRulesToggleCmd)
	eventbusRulesCmd.AddCommand(eventbusRulesImportCmd)

	eventbusCmd.AddCommand(eventbusStopCmd)
	eventbusCmd.AddCommand(eventbusLogsCmd)
	eventbusCmd.AddCommand(eventbusRulesCmd)
	eventbusCmd.AddCommand(eventbusMonitorCmd)
	eventbusCmd.AddCommand(eventbusStatusCmd)
	eventbusCmd.AddCommand(eventbusReplayCmd)
	eventbusCmd.AddCommand(eventbusDLQCmd)

	rootCmd.AddCommand(eventbusCmd)
}

// --- Shared helpers ---

func resolveEventbusDataDir() (string, error) {
	if eventbusDataDir != "" {
		return eventbusDataDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".c4", "eventbus"), nil
}

func resolveSocketPath(dataDir string) string {
	if eventbusSocket != "" {
		return eventbusSocket
	}
	return filepath.Join(dataDir, "c3.sock")
}

func connectEventbus() (*eventbus.Client, error) {
	dataDir, err := resolveEventbusDataDir()
	if err != nil {
		return nil, err
	}
	sockPath := resolveSocketPath(dataDir)
	return eventbus.NewClient(sockPath)
}

func parseSinceTime(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try date only
		t, err = time.Parse("2006-01-02", s)
		if err != nil {
			return 0
		}
	}
	return t.UnixMilli()
}

// --- Daemon command ---

func runEventbus(cmd *cobra.Command, args []string) error {
	dataDir, err := resolveEventbusDataDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	sockPath := resolveSocketPath(dataDir)

	// PID file lock
	pidPath := filepath.Join(dataDir, "eventbus.pid")
	if err := acquireEventbusPIDLock(pidPath); err != nil {
		return err
	}
	defer os.Remove(pidPath)

	// Remove stale socket file before listening
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	// Open store
	dbPath := filepath.Join(dataDir, "events.db")
	store, err := eventbus.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Load default rules
	defaultRulesPath := filepath.Join(dataDir, "default_rules.yaml")
	if data, err := os.ReadFile(defaultRulesPath); err == nil {
		if err := store.EnsureDefaultRules(data); err != nil {
			fmt.Fprintf(os.Stderr, "cq eventbus: default rules: %v\n", err)
		}
	}

	// Dispatcher
	dispatcher := eventbus.NewDispatcher(store)

	// gRPC server
	srv := eventbus.NewServer(eventbus.ServerConfig{
		Store:      store,
		Dispatcher: dispatcher,
	})

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", sockPath, err)
	}
	defer os.Remove(sockPath)

	grpcServer := grpc.NewServer()
	pb.RegisterEventBusServer(grpcServer, srv)

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\ncq eventbus: shutting down (signal)...")
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "cq eventbus: shutting down...")
		}
		grpcServer.GracefulStop()
	}()

	fmt.Fprintf(os.Stderr, "cq eventbus: listening on unix:%s (data: %s)\n", sockPath, dataDir)

	if err := grpcServer.Serve(ln); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	fmt.Fprintln(os.Stderr, "cq eventbus: stopped")
	return nil
}

func runEventbusStop(cmd *cobra.Command, args []string) error {
	dataDir, err := resolveEventbusDataDir()
	if err != nil {
		return err
	}

	pidPath := filepath.Join(dataDir, "eventbus.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("eventbus not running (no PID file at %s)", pidPath)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to signal process %d: %w", pid, err)
	}

	fmt.Printf("cq eventbus: sent SIGTERM to PID %d\n", pid)
	return nil
}

// --- Logs command ---

func runEventbusLogs(cmd *cobra.Command, args []string) error {
	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w (is daemon running?)", err)
	}
	defer client.Close()

	sinceMs := parseSinceTime(logsSince)

	logs, err := client.ListLogs("", logsLimit, sinceMs, logsType)
	if err != nil {
		return fmt.Errorf("list logs: %w", err)
	}

	if len(logs) == 0 {
		fmt.Println("No dispatch logs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tEVENT_TYPE\tRULE\tSTATUS\tDURATION\tERROR")
	for _, l := range logs {
		ts := time.UnixMilli(l.TimestampMs).Format("15:04:05")
		dur := fmt.Sprintf("%dms", l.DurationMs)
		errStr := l.Error
		if len(errStr) > 50 {
			errStr = errStr[:50] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			ts, l.EventType, l.RuleName, l.Status, dur, errStr)
	}
	w.Flush()
	fmt.Printf("\nTotal: %d entries\n", len(logs))
	return nil
}

// --- Rules commands ---

func runRulesList(cmd *cobra.Command, args []string) error {
	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w (is daemon running?)", err)
	}
	defer client.Close()

	rules, err := client.ListRules()
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}

	if len(rules) == 0 {
		fmt.Println("No rules configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATTERN\tACTION\tENABLED\tPRIORITY")
	for _, r := range rules {
		enabled := "yes"
		if !r.Enabled {
			enabled = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n",
			r.Name, r.EventPattern, r.ActionType, enabled, r.Priority)
	}
	w.Flush()
	fmt.Printf("\nTotal: %d rules\n", len(rules))
	return nil
}

func runRulesAdd(cmd *cobra.Command, args []string) error {
	if ruleAddName == "" || ruleAddPattern == "" || ruleAddAction == "" {
		return fmt.Errorf("--name, --pattern, and --action are required")
	}

	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	ruleID, err := client.AddRule(ruleAddName, ruleAddPattern, ruleAddFilter, ruleAddAction, ruleAddConfig, ruleAddEnabled, ruleAddPriority)
	if err != nil {
		return fmt.Errorf("add rule: %w", err)
	}

	fmt.Printf("Rule added: %s (id: %s)\n", ruleAddName, ruleID)
	return nil
}

func runRulesRemove(cmd *cobra.Command, args []string) error {
	if ruleRemoveName == "" {
		return fmt.Errorf("--name is required")
	}

	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	if err := client.RemoveRule("", ruleRemoveName); err != nil {
		return fmt.Errorf("remove rule: %w", err)
	}

	fmt.Printf("Rule removed: %s\n", ruleRemoveName)
	return nil
}

func runRulesToggle(cmd *cobra.Command, args []string) error {
	if ruleToggleName == "" {
		return fmt.Errorf("--name is required")
	}

	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	if err := client.ToggleRule(ruleToggleName, ruleToggleEnabled); err != nil {
		return fmt.Errorf("toggle rule: %w", err)
	}

	state := "enabled"
	if !ruleToggleEnabled {
		state = "disabled"
	}
	fmt.Printf("Rule %s: %s\n", ruleToggleName, state)
	return nil
}

func runRulesImport(cmd *cobra.Command, args []string) error {
	if ruleImportFile == "" {
		return fmt.Errorf("--file is required")
	}

	data, err := os.ReadFile(ruleImportFile)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	type ruleYAML struct {
		Name         string `yaml:"name"`
		EventPattern string `yaml:"event_pattern"`
		FilterJSON   string `yaml:"filter_json"`
		ActionType   string `yaml:"action_type"`
		ActionConfig string `yaml:"action_config"`
		Enabled      bool   `yaml:"enabled"`
		Priority     int    `yaml:"priority"`
	}
	type rulesFile struct {
		Rules []ruleYAML `yaml:"rules"`
	}

	var rf rulesFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}

	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	// Get existing rules to skip duplicates
	existingRules, _ := client.ListRules()
	existingNames := make(map[string]bool, len(existingRules))
	for _, r := range existingRules {
		existingNames[r.Name] = true
	}

	var added, skipped, failed int
	for _, r := range rf.Rules {
		if existingNames[r.Name] {
			fmt.Printf("  skipped: %s (already exists)\n", r.Name)
			skipped++
			continue
		}
		_, err := client.AddRule(r.Name, r.EventPattern, r.FilterJSON, r.ActionType, r.ActionConfig, r.Enabled, r.Priority)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed: %s (%v)\n", r.Name, err)
			failed++
		} else {
			fmt.Printf("  added: %s\n", r.Name)
			added++
		}
	}

	fmt.Printf("\nImported %d rules (%d skipped, %d failed)\n", added, skipped, failed)
	return nil
}

// --- Monitor command ---

func runMonitor(cmd *cobra.Command, args []string) error {
	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	fmt.Fprintf(os.Stderr, "cq eventbus: monitoring events (pattern: %s) — Ctrl+C to stop\n", monitorPattern)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	ch, err := client.Subscribe(ctx, monitorPattern, "")
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	for ev := range ch {
		ts := time.UnixMilli(ev.TimestampMs).Format("15:04:05.000")
		dataStr := string(ev.Data)
		if len(dataStr) > 100 {
			dataStr = dataStr[:100] + "..."
		}
		corrStr := ""
		if ev.CorrelationId != "" {
			corrStr = fmt.Sprintf(" corr=%s", ev.CorrelationId)
		}
		fmt.Printf("[%s] %s (src=%s%s) %s\n", ts, ev.Type, ev.Source, corrStr, dataStr)
	}

	return nil
}

// --- Status command ---

func runEventbusStatus(cmd *cobra.Command, args []string) error {
	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w (is daemon running?)", err)
	}
	defer client.Close()

	stats, err := client.GetStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	dataDir, _ := resolveEventbusDataDir()
	sockPath := resolveSocketPath(dataDir)

	fmt.Println("C3 EventBus Status")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Socket:       %s\n", sockPath)
	fmt.Printf("Events:       %d\n", stats.EventCount)
	fmt.Printf("Rules:        %d\n", stats.RuleCount)
	fmt.Printf("Log entries:  %d\n", stats.LogCount)
	if stats.OldestEvent != "" {
		fmt.Printf("Oldest event: %s\n", stats.OldestEvent)
	}
	if stats.NewestEvent != "" {
		fmt.Printf("Newest event: %s\n", stats.NewestEvent)
	}
	return nil
}

// --- Replay command ---

func runReplay(cmd *cobra.Command, args []string) error {
	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	sinceMs := parseSinceTime(replaySince)

	mode := "dry-run"
	if !replayDryRun {
		mode = "re-dispatch"
	}
	fmt.Fprintf(os.Stderr, "cq eventbus: replaying events (type=%q, mode=%s, limit=%d)\n",
		replayType, mode, replayLimit)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ch, err := client.ReplayEvents(ctx, replayType, sinceMs, replayLimit, replayDryRun)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}

	count := 0
	for ev := range ch {
		count++
		ts := time.UnixMilli(ev.TimestampMs).Format("15:04:05")
		var data map[string]any
		json.Unmarshal(ev.Data, &data)
		fmt.Printf("[%s] %s (id=%s)\n", ts, ev.Type, ev.Id)
	}

	fmt.Printf("\nReplayed %d events\n", count)
	return nil
}

// --- DLQ command ---

func runDLQ(cmd *cobra.Command, args []string) error {
	client, err := connectEventbus()
	if err != nil {
		return fmt.Errorf("connect: %w (is daemon running?)", err)
	}
	defer client.Close()

	entries, err := client.ListDLQ(dlqLimit)
	if err != nil {
		return fmt.Errorf("list dlq: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("Dead Letter Queue is empty.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEVENT_TYPE\tRULE\tRETRIES\tERROR\tCREATED")
	for _, e := range entries {
		ts := time.UnixMilli(e.CreatedAtMs).Format("2006-01-02 15:04:05")
		errStr := e.Error
		if len(errStr) > 50 {
			errStr = errStr[:50] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%d/%d\t%s\t%s\n",
			e.Id, e.EventType, e.RuleName, e.RetryCount, e.MaxRetries, errStr, ts)
	}
	w.Flush()
	fmt.Printf("\nTotal: %d entries\n", len(entries))
	return nil
}

// acquireEventbusPIDLock writes the current PID to a file and checks for existing daemon.
func acquireEventbusPIDLock(pidPath string) error {
	if data, err := os.ReadFile(pidPath); err == nil {
		pid, err := strconv.Atoi(string(data))
		if err == nil {
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("eventbus already running (PID %d). Stop it with: cq eventbus stop", pid)
				}
			}
		}
		os.Remove(pidPath)
	}

	return os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}
