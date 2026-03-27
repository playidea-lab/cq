//go:build hub

package main

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
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/notify"
	"github.com/changmin/c4-core/internal/serve"
)

// registerWorkerServeComponent registers the Worker component when
// the hub build tag is active and worker is enabled.
// Checks: --worker flag, worker.enabled config, hub.auto_worker (legacy compat).
func registerWorkerServeComponent(mgr *serve.Manager, cfg config.C4Config, hubClientAny any) {
	enabled := serveWorker || cfg.Worker.Enabled || cfg.Hub.AutoWorker
	if !enabled {
		return
	}

	hubClient, ok := hubClientAny.(*hub.Client)
	if !ok || hubClient == nil {
		fmt.Fprintln(os.Stderr, "cq serve: worker requested but hub client not available")
		return
	}

	hostname, _ := os.Hostname()

	// Tags: worker.tags > hub.worker_tags > default
	tags := cfg.Worker.Tags
	if len(tags) == 0 {
		tags = cfg.Hub.WorkerTags
	}
	if len(tags) == 0 {
		tags = []string{"cq-worker"}
	}

	comp := serve.NewWorker(hubClient, tags, hostname)

	// Wire Telegram notification for job completion.
	// Credentials are resolved from .c4/notifications.json + botstore.
	// If either is missing, skip notification (graceful degradation).
	notifPath := filepath.Join(projectDir, ".c4", "notifications.json")
	data, err := os.ReadFile(notifPath)
	if err == nil {
		var notifCfg struct {
			BotUsername string `json:"bot_username"`
		}
		if json.Unmarshal(data, &notifCfg) == nil && notifCfg.BotUsername != "" {
			bs, bsErr := botstore.New(projectDir)
			if bsErr == nil {
				bot, botErr := bs.Get(notifCfg.BotUsername)
				if botErr == nil && len(bot.AllowFrom) > 0 {
					token := bot.Token
					chatID := strconv.FormatInt(bot.AllowFrom[0], 10)
					comp.SetNotifyFunc(func(jobID, status string, exitCode int) {
						emoji := "✅"
						if status == "FAILED" {
							emoji = "❌"
						}
						msg := fmt.Sprintf("%s Job `%s` %s (exit=%d) on %s", emoji, jobID, status, exitCode, hostname)
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						if err := notify.SendTelegram(ctx, token, chatID, msg); err != nil {
							fmt.Fprintf(os.Stderr, "cq serve: telegram notify: %v\n", err)
						}
					})
				} else if !errors.Is(botErr, botstore.ErrNotFound) && botErr != nil {
					fmt.Fprintf(os.Stderr, "cq serve: worker notify: botstore lookup: %v\n", botErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "cq serve: worker notify: botstore init: %v\n", bsErr)
			}
		}
	}

	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered worker (tags=%v)\n", tags)
}
