//go:build c3_eventbus

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp/handlers"
	"github.com/changmin/c4-core/internal/mcp/handlers/eventbushandler"
	"github.com/changmin/c4-core/internal/mcp/handlers/knowledgehandler"
	"github.com/changmin/c4-core/internal/notify"
	"github.com/changmin/c4-core/internal/serve"
)

func init() {
	registerInitHook(initEventBus)
	registerShutdownHook(shutdownEventBus)
}

// initEventBus starts the embedded or remote EventBus and wires all subscribers.
func initEventBus(ctx *initContext) error {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().EventBus.Enabled {
		wireLocalDispatcher(ctx)
		return nil
	}
	ebCfg := ctx.cfgMgr.GetConfig().EventBus

	// If cq serve is running, skip embedded and connect to its remote EventBus.
	// This avoids socket conflicts since serve already manages the EventBus.
	if ebCfg.AutoStart && serve.IsServeRunning() {
		fmt.Fprintln(os.Stderr, "cq: serve running, using remote eventbus")
		initRemoteEB(ctx)
	} else if ebCfg.AutoStart {
		initEmbeddedEB(ctx)
	} else {
		initRemoteEB(ctx)
	}

	// Wire local Dispatcher for C1 posting if no embedded server was started
	wireLocalDispatcher(ctx)
	return nil
}

// initEmbeddedEB starts the in-process EventBus and wires all components.
func initEmbeddedEB(ctx *initContext) {
	ebCfg := ctx.cfgMgr.GetConfig().EventBus
	dataDir := ebCfg.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(ctx.projectDir, ".c4", "eventbus")
	}
	defaultRulesPath := filepath.Join(ctx.projectDir, "c4-core", "internal", "eventbus", "default_rules.yaml")
	if _, err := os.Stat(defaultRulesPath); err != nil {
		defaultRulesPath = filepath.Join(dataDir, "default_rules.yaml")
		if _, err := os.Stat(defaultRulesPath); err != nil {
			defaultRulesPath = ""
		}
	}

	eb, ebErr := eventbus.StartEmbedded(eventbus.EmbeddedConfig{
		DataDir:          dataDir,
		RetentionDays:    ebCfg.RetentionDays,
		MaxEvents:        ebCfg.MaxEvents,
		DefaultRulesPath: defaultRulesPath,
		WSPort:           ebCfg.WSPort,
		WSHost:           ebCfg.WSHost,
	})
	if ebErr != nil {
		fmt.Fprintf(os.Stderr, "cq: eventbus auto-start failed: %v\n", ebErr)
		return
	}
	ctx.embeddedEB = eb

	if ctx.keeper != nil {
		eb.Dispatcher().SetC1Poster(ctx.keeper)
	}

	wireTelegramSender(eb.Dispatcher(), ctx.projectDir)

	ebClient, ebErr := eventbus.NewClient(eb.SocketPath())
	if ebErr != nil {
		return
	}

	wireAllEventBus(ctx, ebClient)
	eventbushandler.RegisterEventBusHandlers(ctx.reg, ebClient, ctx.cfgMgr)
	ctx.sqliteStore.SetEventBus(ebClient)
	ctx.proxy.SetEventBus(ebClient)

	ctx.sqliteStore.SetDispatcher(eb.Dispatcher())

	if js := hubJobSubmitter(ctx); js != nil {
		eb.Dispatcher().SetHubSubmitter(js)
	}

	fmt.Fprintf(os.Stderr, "cq: eventbus auto-started (embedded, %s)\n", eb.SocketPath())
}

// initRemoteEB connects to a remote EventBus daemon.
func initRemoteEB(ctx *initContext) {
	ebCfg := ctx.cfgMgr.GetConfig().EventBus
	sockPath := ebCfg.SocketPath
	if sockPath == "" {
		home, _ := os.UserHomeDir()
		sockPath = filepath.Join(home, ".c4", "eventbus", "c3.sock")
	}
	ebClient, ebErr := eventbus.NewClient(sockPath)
	if ebErr != nil {
		fmt.Fprintf(os.Stderr, "cq: eventbus not reachable (unix:%s): %v\n", sockPath, ebErr)
		return
	}

	wireAllEventBus(ctx, ebClient)
	eventbushandler.RegisterEventBusHandlers(ctx.reg, ebClient, ctx.cfgMgr)
	ctx.sqliteStore.SetEventBus(ebClient)
	ctx.proxy.SetEventBus(ebClient)

	fmt.Fprintf(os.Stderr, "cq: eventbus connected (unix:%s, 7 tools)\n", sockPath)
}

// wireAllEventBus fires all component EB wiring hooks plus core wiring.
func wireAllEventBus(ctx *initContext, ebClient *eventbus.Client) {
	ctx.ebClient = ebClient // stored for serve-time components (e.g. LoopOrchestrator)
	for _, fn := range componentEBWireHooks {
		fn(ctx, ebClient)
	}
	// Core wiring (always active, not build-tagged)
	handlers.SetValidationEventBus(ebClient)
	knowledgehandler.SetKnowledgeEventBus(ebClient, ctx.sqliteStore.GetProjectID())
}

// wireTelegramSender resolves the notification bot from .c4/notifications.json
// and wires a TelegramSender into the dispatcher.
func wireTelegramSender(dispatcher *eventbus.Dispatcher, projectDir string) {
	cfgPath := filepath.Join(projectDir, ".c4", "notifications.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return // not configured
	}
	var cfg struct {
		BotUsername string `json:"bot_username"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.BotUsername == "" {
		return
	}
	bs, err := botstore.New(projectDir)
	if err != nil {
		return
	}
	bot, err := bs.Get(cfg.BotUsername)
	if err != nil || bot.Token == "" {
		return
	}
	dispatcher.SetTelegramSender(&notify.BotSender{Token: bot.Token})
	fmt.Fprintf(os.Stderr, "cq: telegram sender wired (bot=%s)\n", cfg.BotUsername)
}

// telegramChatID resolves the default chat_id from the notification bot's AllowFrom.
func telegramChatID(projectDir string) string {
	cfgPath := filepath.Join(projectDir, ".c4", "notifications.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}
	var cfg struct {
		BotUsername string `json:"bot_username"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.BotUsername == "" {
		return ""
	}
	bs, err := botstore.New(projectDir)
	if err != nil {
		return ""
	}
	bot, err := bs.Get(cfg.BotUsername)
	if err != nil || len(bot.AllowFrom) == 0 {
		return ""
	}
	return strconv.FormatInt(bot.AllowFrom[0], 10)
}

// wireLocalDispatcher creates a minimal local dispatcher for C1 posting
// when no EventBus is available.
func wireLocalDispatcher(ctx *initContext) {
	if ctx.embeddedEB != nil {
		return
	}
	if ctx.keeper == nil {
		// Even without C1, wire telegram if configured
		wireLocalTelegramDispatcher(ctx)
		return
	}
	localDBPath := filepath.Join(ctx.projectDir, ".c4", "eventbus", "local.db")
	localStore, localErr := eventbus.NewStore(localDBPath)
	if localErr != nil {
		fmt.Fprintf(os.Stderr, "cq: local eventbus store failed: %v\n", localErr)
		return
	}
	localDispatcher := eventbus.NewDispatcher(localStore)
	localDispatcher.SetC1Poster(ctx.keeper)
	wireTelegramSender(localDispatcher, ctx.projectDir)

	rules, _ := localStore.MatchRules("task.completed")
	if len(rules) == 0 {
		localStore.AddRule(
			"c1-task-updates",
			"task.*",
			"",
			"c1_post",
			`{"channel":"#updates","template":"[{{event_type}}] {{task_id}}: {{title}}"}`,
			true,
			0,
		)
	}

	addTelegramRuleIfNeeded(localStore, ctx.projectDir)

	ctx.sqliteStore.SetDispatcher(localDispatcher)
	fmt.Fprintln(os.Stderr, "cq: local dispatcher wired (c1_post + telegram rules)")
}

// wireLocalTelegramDispatcher creates a minimal dispatcher with only telegram rules
// when no C1 keeper is available but telegram is configured.
func wireLocalTelegramDispatcher(ctx *initContext) {
	chatID := telegramChatID(ctx.projectDir)
	if chatID == "" {
		return
	}
	localDBPath := filepath.Join(ctx.projectDir, ".c4", "eventbus", "local.db")
	localStore, err := eventbus.NewStore(localDBPath)
	if err != nil {
		return
	}
	localDispatcher := eventbus.NewDispatcher(localStore)
	wireTelegramSender(localDispatcher, ctx.projectDir)
	addTelegramRuleIfNeeded(localStore, ctx.projectDir)
	ctx.sqliteStore.SetDispatcher(localDispatcher)
	fmt.Fprintln(os.Stderr, "cq: local dispatcher wired (telegram rules)")
}

// addTelegramRuleIfNeeded adds default telegram notification rules if a telegram
// bot is configured and no telegram rules exist yet.
func addTelegramRuleIfNeeded(store *eventbus.Store, projectDir string) {
	chatID := telegramChatID(projectDir)
	if chatID == "" {
		return
	}

	// Check if telegram rules already exist
	rules, _ := store.ListRules()
	for _, r := range rules {
		if r.ActionType == "telegram" {
			return // already has telegram rules
		}
	}

	// Add default telegram rules for important events
	templateCfg := func(tmpl string) string {
		return fmt.Sprintf(`{"chat_id":"%s","template":"%s"}`, chatID, tmpl)
	}

	store.AddRule("telegram-task-updates", "task.*", "", "telegram",
		templateCfg("[{{event_type}}] {{task_id}}: {{title}}"), true, 50)
	store.AddRule("telegram-checkpoint-events", "checkpoint.*", "", "telegram",
		templateCfg("[checkpoint] {{decision}}: {{checkpoint_id}}"), true, 50)
	store.AddRule("telegram-hub-events", "hub.*", "", "telegram",
		templateCfg("[hub] {{event_type}}: {{job_id}} {{name}}"), true, 50)
}

// startEventSink starts the EventSink HTTP server if configured.
func startEventSink(ctx *initContext) {
	if ctx.cfgMgr == nil || !ctx.cfgMgr.GetConfig().EventSink.Enabled {
		return
	}
	if serve.IsServeRunning() {
		fmt.Fprintln(os.Stderr, serve.StatusMessage("eventsink"))
		return
	}
	esCfg := ctx.cfgMgr.GetConfig().EventSink
	hubEventPub := handlers.GetHubEventPub()
	esSrv, esErr := eventbushandler.StartEventSinkServer(esCfg.Port, esCfg.Token, hubEventPub)
	if esErr != nil {
		fmt.Fprintf(os.Stderr, "cq: eventsink start failed: %v\n", esErr)
		return
	}
	if esSrv != nil {
		ctx.eventsinkSrv = esSrv
		fmt.Fprintf(os.Stderr, "cq: eventsink listening on :%d\n", esCfg.Port)
	}
}

// shutdownEventBus cleans up EventBus resources.
func shutdownEventBus(ctx *initContext) {
	if ctx.eventsinkSrv != nil {
		ctx.eventsinkSrv.Close()
	}
	if ctx.embeddedEB != nil {
		ctx.embeddedEB.Stop()
	}
}
