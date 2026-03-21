package harness

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/changmin/c4-core/internal/channelpush"
	"github.com/changmin/c4-core/internal/observe"
)

// TraceRecorder is a local interface for recording LLM trace steps.
// observe.TraceCollector satisfies this interface.
type TraceRecorder interface {
	EnsureTrace(traceID string)
	AddStep(traceID string, step observe.TraceStep)
}

var globalTraceRecorderMu sync.RWMutex
var globalTraceRecorder TraceRecorder

// SetTraceRecorder installs a TraceRecorder for use in readNewLines.
// Passing nil disables trace recording.
func SetTraceRecorder(r TraceRecorder) {
	globalTraceRecorderMu.Lock()
	globalTraceRecorder = r
	globalTraceRecorderMu.Unlock()
}

// JournalWatcher watches ~/.claude/projects/{slug}/*.jsonl for new lines
// and pushes them to c1_channels via ChannelPusher.
type JournalWatcher struct {
	syncer       *SyncPusher
	positions    *PositionStore
	watcher      *fsnotify.Watcher
	// projectsRoot is overridable in tests; defaults to ~/.claude/projects.
	projectsRoot string
}

// NewJournalWatcher creates a JournalWatcher.
func NewJournalWatcher(pusher ChannelPusher, positions *PositionStore, tenantID string) *JournalWatcher {
	var syncer *SyncPusher
	if pusher != nil {
		syncer = newSyncPusher(pusher, tenantID)
	}
	return &JournalWatcher{
		syncer:    syncer,
		positions: positions,
	}
}

// Start begins watching the projects directory. Returns nil if the directory
// does not exist yet (graceful — fsnotify will watch once it appears via parent).
func (w *JournalWatcher) Start(ctx context.Context) error {
	if w.syncer == nil {
		log.Println("[journal_watcher] pusher not configured, skipping")
		return nil
	}

	root := w.projectsRoot
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = filepath.Join(home, ".claude", "projects")
		w.projectsRoot = root
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	// Watch root directory for new slug dirs.
	if err := watcher.Add(root); err != nil {
		log.Printf("[journal_watcher] projects dir not watchable: %v", err)
	}

	// Walk existing slug dirs (depth=1) and cold scan.
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slugDir := filepath.Join(root, entry.Name())
		_ = watcher.Add(slugDir)
		w.coldScan(ctx, slugDir)
	}

	go w.watch(ctx, root)
	return nil
}

func (w *JournalWatcher) watch(ctx context.Context, projectsRoot string) {
	defer w.watcher.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// New slug directory created under projects root.
			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && filepath.Dir(event.Name) == projectsRoot {
					_ = w.watcher.Add(event.Name)
					continue
				}
			}
			// JSONL file written or created.
			if (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) &&
				strings.HasSuffix(event.Name, ".jsonl") {
				w.processFile(ctx, event.Name)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[journal_watcher] watcher error: %v", err)
		}
	}
}

func (w *JournalWatcher) coldScan(ctx context.Context, slugDir string) {
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			w.processFile(ctx, filepath.Join(slugDir, e.Name()))
		}
	}
}

func (w *JournalWatcher) processFile(ctx context.Context, filePath string) {
	offset := w.positions.GetOffset(filePath)
	msgs, newOffset := readNewLines(filePath, offset)
	if len(msgs) == 0 {
		return
	}
	if err := w.syncer.Push(ctx, filePath, msgs); err != nil {
		log.Printf("[journal_watcher] push error for %s: %v", filePath, err)
		return // offset NOT updated -- will retry on next fsnotify event
	}
	_ = w.positions.SetOffset(filePath, newOffset)
}

// Stop closes the fsnotify watcher.
func (w *JournalWatcher) Stop(_ context.Context) error {
	if w.watcher != nil {
		return w.watcher.Close()
	}
	return nil
}

// readNewLines reads new lines from filePath starting at byteOffset.
// Returns parsed messages and the new file size as the next offset.
func readNewLines(filePath string, offset int64) ([]channelpush.PushMessage, int64) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, offset
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, offset
	}
	newOffset := info.Size()
	if newOffset <= offset {
		return nil, offset
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset
	}

	buf := make([]byte, newOffset-offset)
	if _, err := f.Read(buf); err != nil {
		return nil, offset
	}

	sessionUUID := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")

	globalTraceRecorderMu.RLock()
	recorder := globalTraceRecorder
	globalTraceRecorderMu.RUnlock()

	var msgs []channelpush.PushMessage
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineBytes := []byte(line)
		msg, _ := ParseClaudeCodeLine(lineBytes)
		if msg != nil {
			msgs = append(msgs, *msg)
		}
		if recorder != nil {
			if info, err := ExtractUsage(lineBytes); err == nil && info != nil {
				recorder.EnsureTrace(sessionUUID)
				recorder.AddStep(sessionUUID, observe.TraceStep{
					TraceID:   sessionUUID,
					StepType:  observe.StepTypeLLM,
					Timestamp: time.Now(),
					Provider:  info.Provider,
					Model:     info.Model,
					InputTok:  int64(info.InputTok),
					OutputTok: int64(info.OutputTok),
					Success:   true,
				})
			}
		}
	}
	return msgs, newOffset
}

// filePathToChannelName converts a .jsonl file path to a channel name.
// e.g. /path/to/{sessionUUID}.jsonl → "claude_code:{sessionUUID}"
func filePathToChannelName(filePath string) string {
	base := filepath.Base(filePath)
	sessionUUID := strings.TrimSuffix(base, ".jsonl")
	return "claude_code:" + sessionUUID
}
