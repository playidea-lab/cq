package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/notify"
)

// taskNotifyConfig mirrors the shape of .c4/notifications.json.
type taskNotifyConfig struct {
	BotUsername string `json:"bot_username"`
}

// handleTaskEvent processes c4_tasks Realtime events and sends Telegram notifications
// when a task transitions to "done" or "blocked".
func (a *Agent) handleTaskEvent(event RealtimeEvent) {
	if event.ChangeType != "UPDATE" {
		return
	}

	var record struct {
		TaskID           string `json:"task_id"`
		Title            string `json:"title"`
		Status           string `json:"status"`
		FailureSignature string `json:"failure_signature"`
	}
	if err := json.Unmarshal(event.Record, &record); err != nil {
		return
	}

	var message string
	switch record.Status {
	case "done":
		message = fmt.Sprintf("✅ %s: %s", record.TaskID, record.Title)
	case "blocked":
		reason := record.FailureSignature
		if reason == "" {
			reason = "unknown"
		}
		message = fmt.Sprintf("🚫 %s blocked: %s", record.TaskID, reason)
	case "in_progress":
		// No telegram for in_progress, but still write event file
	default:
		return // skip pending, etc.
	}

	// Write event file for /c4-run polling
	a.writeTaskEvent(record.TaskID, record.Status, record.Title)

	if message != "" {
		a.sendTaskNotification(message)
	}
}

// writeTaskEvent writes a JSON event file to .c4/events/ for /c4-run to consume.
func (a *Agent) writeTaskEvent(taskID, status, title string) {
	a.mu.Lock()
	projectDir := a.cfg.ProjectDir
	a.mu.Unlock()
	if projectDir == "" {
		return
	}

	eventsDir := filepath.Join(projectDir, ".c4", "events")
	os.MkdirAll(eventsDir, 0755)

	event := map[string]string{
		"task_id":   taskID,
		"status":    status,
		"title":     title,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(event)

	// Filename includes task_id and status for easy glob matching
	filename := fmt.Sprintf("task-%s-%s.json", taskID, status)
	path := filepath.Join(eventsDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] write task event: %v\n", err)
	}
}

// sendTaskNotification sends a Telegram message using the notification config from
// .c4/notifications.json (same file used by c4_notify). Best-effort: errors are
// logged to stderr and do not affect the caller.
func (a *Agent) sendTaskNotification(message string) {
	a.mu.Lock()
	projectDir := a.cfg.ProjectDir
	a.mu.Unlock()

	if projectDir == "" {
		return
	}

	// Load notification config
	cfgPath := filepath.Join(projectDir, ".c4", "notifications.json")
	data, err := os.ReadFile(cfgPath)
	if errors.Is(err, os.ErrNotExist) {
		return // notifications not configured — silently skip
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] task notify: read config: %v\n", err)
		return
	}

	var cfg taskNotifyConfig
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.BotUsername == "" {
		return
	}

	// Resolve bot credentials from botstore
	bs, err := botstore.New(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] task notify: botstore: %v\n", err)
		return
	}
	bot, err := bs.Get(cfg.BotUsername)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] task notify: bot %q not found: %v\n", cfg.BotUsername, err)
		return
	}
	if len(bot.AllowFrom) == 0 {
		fmt.Fprintf(os.Stderr, "cq: [agent] task notify: bot %q has no AllowFrom entries\n", cfg.BotUsername)
		return
	}
	chatID := strconv.FormatInt(bot.AllowFrom[0], 10)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := notify.SendTelegram(ctx, bot.Token, chatID, message); err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] task notify: telegram send: %v\n", err)
	}
}
