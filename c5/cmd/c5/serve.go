package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	c5 "github.com/piqsol/c4/c5"
	"github.com/piqsol/c4/c5/internal/api"
	"github.com/piqsol/c4/c5/internal/config"
	"github.com/piqsol/c4/c5/internal/storage"
	"github.com/piqsol/c4/c5/internal/store"
	"github.com/spf13/cobra"
)

func serveCmd() *cobra.Command {
	var (
		configPath  string
		printConfig bool
		port        int
		dbPath      string
		apiKey      string
		eventBusURL   string
		eventBusToken string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the C5 job queue server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if printConfig {
				fmt.Print(config.ExampleConfigYAML())
				return nil
			}
			return runServe(cmd, configPath, port, dbPath, apiKey, eventBusURL, eventBusToken)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to config file (default: ~/.config/c5/c5.yaml)")
	cmd.Flags().BoolVar(&printConfig, "print-config", false, "Print example config YAML and exit")
	cmd.Flags().IntVar(&port, "port", 0, "HTTP port to listen on (overrides config)")
	cmd.Flags().StringVar(&dbPath, "db", defaultDBPath(), "SQLite database path")
	cmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("C5_API_KEY"), "API key for authentication (optional)")
	cmd.Flags().StringVar(&eventBusURL, "eventbus-url", "", "C3 EventBus base URL (overrides config and env)")
	cmd.Flags().StringVar(&eventBusToken, "eventbus-token", "", "Bearer token for EventBus (overrides config and env)")

	return cmd
}

func runServe(cmd *cobra.Command, configPath string, port int, dbPath, apiKey, eventBusURL, eventBusToken string) error {
	// 1. Load config file (explicit path errors if missing; default path is silently ignored)
	explicitConfig := cmd.Flags().Changed("config")
	var cfg *config.Config
	var err error
	if explicitConfig {
		// explicit --config: error if file not found
		cfg, err = loadConfigStrict(configPath)
		if err != nil {
			return err
		}
	} else {
		// default path: silently ignore missing file
		cfg, err = config.Load("")
		if err != nil {
			return err
		}
	}

	// 2. Environment variable overrides
	if v := os.Getenv("C5_EVENTBUS_URL"); v != "" {
		cfg.EventBus.URL = v
	}
	if v := os.Getenv("C5_EVENTBUS_TOKEN"); v != "" {
		cfg.EventBus.Token = v
	}
	if v := os.Getenv("C5_STORAGE_PATH"); v != "" {
		cfg.Storage.Path = v
	}
	if v := os.Getenv("C5_SUPABASE_URL"); v != "" {
		cfg.Storage.SupabaseURL = v
	}
	if v := os.Getenv("C5_SUPABASE_KEY"); v != "" {
		cfg.Storage.SupabaseKey = v
	}

	// 3. CLI flag overrides (only if explicitly specified)
	if cmd.Flags().Changed("eventbus-url") {
		cfg.EventBus.URL = eventBusURL
	}
	if cmd.Flags().Changed("eventbus-token") {
		cfg.EventBus.Token = eventBusToken
	}
	if cmd.Flags().Changed("port") {
		cfg.Server.Port = port
	}

	// Use cfg values
	resolvedPort := cfg.Server.Port
	resolvedEventBusURL := cfg.EventBus.URL
	resolvedEventBusToken := cfg.EventBus.Token
	serverURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, resolvedPort)

	// Select storage backend
	var storageBackend storage.Backend
	if cfg.IsSupabaseEnabled() {
		log.Println("c5: using Supabase storage")
		storageBackend = storage.NewSupabase(cfg.Storage.SupabaseURL, cfg.Storage.SupabaseKey)
	} else {
		log.Printf("c5: using local storage at %s", cfg.Storage.Path)
		storageBackend = storage.NewLocal(cfg.Storage.Path, serverURL)
	}

	// Optional: validate/create bucket (non-fatal)
	if bm, ok := storageBackend.(storage.BucketManager); ok {
		if err := bm.EnsureBucket(); err != nil {
			log.Printf("c5: bucket check failed (non-fatal): %v", err)
		}
	}

	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	srv := api.NewServer(api.Config{
		Store:            st,
		Storage:          storageBackend,
		ServerURL:        serverURL,
		Version:          version,
		APIKey:           apiKey,
		LLMSTxt:          c5.LLMSTxt,
		DocsFS:           c5.DocsFS,
		EventBusURL:      resolvedEventBusURL,
		EventBusToken:    resolvedEventBusToken,
		MaxArtifactBytes: cfg.Storage.MaxArtifactBytes,
		GPUWorkerGPUOnly: cfg.Server.GPUWorkerGPUOnly,
	})
	defer srv.Close()

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", resolvedPort),
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute, // generous: large artifact uploads and SSE reconnect within 10m
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("c5: shutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutCtx)
	}()

	log.Printf("c5: serving on :%d (db: %s)", resolvedPort, dbPath)
	if apiKey != "" {
		log.Println("c5: API key authentication enabled")
	}

	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// defaultDBPath returns the default database path under the user's data directory.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./c5.db"
	}
	return filepath.Join(home, ".local", "share", "c5", "c5.db")
}

// loadConfigStrict loads the config from configPath and returns an error if the file does not exist.
func loadConfigStrict(configPath string) (*config.Config, error) {
	_, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config: file not found: %s", configPath)
		}
		return nil, fmt.Errorf("config: stat %q: %w", configPath, err)
	}
	return config.Load(configPath)
}
