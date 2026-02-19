package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	c5 "github.com/piqsol/c4/c5"
	"github.com/piqsol/c4/c5/internal/api"
	"github.com/piqsol/c4/c5/internal/store"
	"github.com/spf13/cobra"
)

func serveCmd() *cobra.Command {
	var (
		port          int
		dbPath        string
		apiKey        string
		eventBusURL   string
		eventBusToken string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the C5 job queue server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(port, dbPath, apiKey, eventBusURL, eventBusToken)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8585, "HTTP port to listen on")
	cmd.Flags().StringVar(&dbPath, "db", "./c5.db", "SQLite database path")
	cmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("C5_API_KEY"), "API key for authentication (optional)")
	cmd.Flags().StringVar(&eventBusURL, "eventbus-url", os.Getenv("C5_EVENTBUS_URL"), "C3 EventBus base URL (optional)")
	cmd.Flags().StringVar(&eventBusToken, "eventbus-token", os.Getenv("C5_EVENTBUS_TOKEN"), "Bearer token for EventBus (optional)")

	return cmd
}

func runServe(port int, dbPath, apiKey, eventBusURL, eventBusToken string) error {
	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	srv := api.NewServer(api.Config{
		Store:         st,
		Version:       version,
		APIKey:        apiKey,
		LLMSTxt:       c5.LLMSTxt,
		DocsFS:        c5.DocsFS,
		EventBusURL:   eventBusURL,
		EventBusToken: eventBusToken,
	})

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
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

	log.Printf("c5: serving on :%d (db: %s)", port, dbPath)
	if apiKey != "" {
		log.Println("c5: API key authentication enabled")
	}

	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
