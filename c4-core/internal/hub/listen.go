// Package hub provides PostgreSQL LISTEN/NOTIFY based real-time job notifications.
// Workers use this as an alternative to polling to receive immediate signals when
// new jobs are queued in Supabase.
//
// Connection requirements:
//   - Must use the direct database URL (port 5432), NOT the pooler (port 6543).
//   - The pooler does not support LISTEN/NOTIFY.
//   - Set cloud.direct_url in config or C4_CLOUD_DIRECT_URL env var.
package hub

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// listenChannel is the PostgreSQL channel name for new job notifications.
	listenChannel = "new_job"

	// reconnectBaseDelay is the initial backoff delay on connection failure.
	reconnectBaseDelay = 1 * time.Second

	// reconnectMaxDelay caps the exponential backoff.
	reconnectMaxDelay = 60 * time.Second
)

// JobNotification is delivered to the handler when a notification arrives
// or when a missed-job poll is required (e.g. after reconnect).
type JobNotification struct {
	// Payload is the raw PostgreSQL NOTIFY payload, typically a job ID or JSON blob.
	// Empty string signals a reconnect-driven poll (all QUEUED jobs should be checked).
	Payload string
}

// NotifyHandler is called by JobListener for each received notification.
// It must be non-blocking or quick: long work should be spawned in a goroutine.
// Return a non-nil error only if the listener should stop.
type NotifyHandler func(n JobNotification) error

// JobListener listens on a PostgreSQL LISTEN channel and delivers notifications
// to a handler. It reconnects automatically with exponential backoff.
type JobListener struct {
	directURL string // direct Postgres connection string (port 5432)
}

// NewJobListener creates a JobListener that connects to the given direct
// database URL. directURL must be a libpq-style DSN or postgres:// URI
// pointing at port 5432 (not the Supabase pooler on port 6543).
func NewJobListener(directURL string) *JobListener {
	return &JobListener{directURL: directURL}
}

// Listen blocks until ctx is cancelled, delivering notifications via handler.
// On each (re)connect it performs a poll by calling handler with an empty
// Payload so the worker can pick up any jobs queued while the connection was
// down. Reconnects use exponential backoff capped at reconnectMaxDelay.
//
// Returns ctx.Err() when the context is cancelled, or the last handler error.
func (l *JobListener) Listen(ctx context.Context, handler NotifyHandler) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := l.listenOnce(ctx, handler)
		if err == nil || ctx.Err() != nil {
			// Normal shutdown or context cancelled.
			return ctx.Err()
		}

		log.Printf("hub/listen: disconnected (attempt %d): %v — reconnecting", attempt+1, err)
		attempt++

		delay := backoffDelay(attempt)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// listenOnce opens one connection, issues LISTEN, delivers notifications until
// the context is cancelled or a connection error occurs.
func (l *JobListener) listenOnce(ctx context.Context, handler NotifyHandler) error {
	conn, err := pgx.Connect(ctx, l.directURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx) //nolint:errcheck

	if _, err := conn.Exec(ctx, "LISTEN "+listenChannel); err != nil {
		return err
	}
	log.Printf("hub/listen: listening on channel %q", listenChannel)

	// On every (re)connect, poll for missed QUEUED jobs.
	if err := handler(JobNotification{Payload: ""}); err != nil {
		return err
	}

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return err
		}
		if err := handler(JobNotification{Payload: notification.Payload}); err != nil {
			return err
		}
	}
}

// backoffDelay returns the exponential delay for the given attempt number (1-based).
func backoffDelay(attempt int) time.Duration {
	d := reconnectBaseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
	if d > reconnectMaxDelay {
		d = reconnectMaxDelay
	}
	return d
}
