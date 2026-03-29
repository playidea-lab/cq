//go:build hub

package serve

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/hub"
)

const (
	watchdogInitialBackoff = 5 * time.Second
	watchdogMaxBackoff     = 5 * time.Minute
	watchdogRingLines      = 500
)

// CrashUploader is the interface Watchdog uses to upload crash logs.
// *hub.Client satisfies this interface.
type CrashUploader interface {
	UploadCrashLog(workerID, content string) error
}

// Watchdog supervises a child "cq serve" process.
// On crash it restarts with exponential backoff (5s→10s→20s→...→5min).
// On clean SIGTERM it forwards the signal to the child and exits.
type Watchdog struct {
	// Args are the arguments passed to the child process (os.Args[0] + serve args).
	Args []string
	// Uploader is called after each crash to upload the last N lines of stderr.
	// May be nil — upload is best-effort.
	Uploader CrashUploader
	// WorkerID identifies this node in crash log uploads.
	WorkerID string
}

// NewWatchdog creates a Watchdog that re-invokes the current binary with serveArgs.
// hubClient may be nil; crash log upload is skipped when nil.
func NewWatchdog(serveArgs []string, hubClient *hub.Client, workerID string) *Watchdog {
	bin, _ := os.Executable()
	args := append([]string{bin}, serveArgs...)
	var uploader CrashUploader
	if hubClient != nil {
		uploader = hubClient
	}
	return &Watchdog{
		Args:     args,
		Uploader: uploader,
		WorkerID: workerID,
	}
}

// Run supervises the child process until ctx is cancelled or a SIGTERM/SIGINT is received.
// It blocks until the child exits cleanly or the context is done.
func (w *Watchdog) Run(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	backoff := watchdogInitialBackoff
	var child *exec.Cmd

	for {
		ring := NewRingBuffer(watchdogRingLines)

		child = exec.CommandContext(ctx, w.Args[0], w.Args[1:]...)
		child.Stdout = os.Stdout
		child.Stdin = os.Stdin

		// Pipe stderr through ring buffer and forward to our stderr.
		stderrPipe, err := child.StderrPipe()
		if err != nil {
			return fmt.Errorf("watchdog: stderr pipe: %w", err)
		}

		if err := child.Start(); err != nil {
			return fmt.Errorf("watchdog: start child: %w", err)
		}
		fmt.Fprintf(os.Stderr, "watchdog: child started (pid=%d)\n", child.Process.Pid)

		// Drain stderr into ring buffer and forward.
		done := make(chan struct{})
		go func() {
			defer close(done)
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				line := scanner.Text()
				ring.Write(line)
				fmt.Fprintln(os.Stderr, line)
			}
		}()

		// Wait for child exit or watchdog signal.
		exitCh := make(chan error, 1)
		go func() {
			exitCh <- child.Wait()
		}()

		var exitErr error
		cleanExit := false

		select {
		case sig := <-sigCh:
			// Forward signal to child; do not restart.
			fmt.Fprintf(os.Stderr, "watchdog: received %s, forwarding to child\n", sig)
			cleanExit = true
			if child.Process != nil {
				_ = child.Process.Signal(sig)
			}
			// Wait for child to exit.
			exitErr = <-exitCh
			<-done
			fmt.Fprintln(os.Stderr, "watchdog: child exited after signal, shutting down")
			return nil

		case <-ctx.Done():
			// Context cancelled — forward SIGTERM.
			cleanExit = true
			if child.Process != nil {
				_ = child.Process.Signal(syscall.SIGTERM)
			}
			exitErr = <-exitCh
			<-done
			fmt.Fprintln(os.Stderr, "watchdog: context done, child exited")
			return ctx.Err()

		case exitErr = <-exitCh:
			<-done
		}

		if cleanExit || exitErr == nil {
			fmt.Fprintln(os.Stderr, "watchdog: child exited cleanly")
			return nil
		}

		// Child crashed — upload log and restart with backoff.
		fmt.Fprintf(os.Stderr, "watchdog: child crashed: %v\n", exitErr)
		w.uploadCrashLog(ring)

		fmt.Fprintf(os.Stderr, "watchdog: restarting in %s\n", backoff)
		select {
		case <-time.After(backoff):
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "watchdog: received %s during backoff, exiting\n", sig)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}

		backoff = nextBackoff(backoff)
	}
}

// nextBackoff doubles the backoff, capped at watchdogMaxBackoff.
func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > watchdogMaxBackoff {
		d = watchdogMaxBackoff
	}
	return d
}

// uploadCrashLog uploads the ring buffer content; logs errors but never returns them.
func (w *Watchdog) uploadCrashLog(ring *RingBuffer) {
	if w.Uploader == nil {
		return
	}
	content := ring.String()
	if content == "" {
		return
	}
	workerID := w.WorkerID
	if workerID == "" {
		host, _ := os.Hostname()
		workerID = host
	}
	if err := w.Uploader.UploadCrashLog(workerID, content); err != nil {
		fmt.Fprintf(os.Stderr, "watchdog: crash log upload failed (best-effort): %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "watchdog: crash log uploaded")
	}
}
