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
		port   int
		dbPath string
		apiKey string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the C5 job queue server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(port, dbPath, apiKey)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8585, "HTTP port to listen on")
	cmd.Flags().StringVar(&dbPath, "db", "./c5.db", "SQLite database path")
	cmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("C5_API_KEY"), "API key for authentication (optional)")

	return cmd
}

func runServe(port int, dbPath, apiKey string) error {
	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	srv := api.NewServer(api.Config{
		Store:   st,
		Version: version,
		APIKey:  apiKey,
		LLMSTxt: c5.LLMSTxt,
		DocsFS:  c5.DocsFS,
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
