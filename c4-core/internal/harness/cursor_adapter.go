package harness

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/changmin/c4-core/internal/c1push"
)

// cursorDBPaths returns candidate paths for Cursor's state.vscdb by OS.
func cursorDBPaths() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")}
	case "linux":
		return []string{filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")}
	default:
		return nil
	}
}

// CursorAdapter polls Cursor's SQLite DB every 5 minutes for new conversations
// and pushes them to c1_channels via ChannelPusher.
type CursorAdapter struct {
	pusher   ChannelPusher
	dbPath   string
	procDB   *sql.DB // cursor_processed table
	done     chan struct{}
	tenantID string
}

// NewCursorAdapter creates a CursorAdapter. Returns nil if Cursor DB not found.
func NewCursorAdapter(pusher ChannelPusher, procDBPath string, tenantID string) *CursorAdapter {
	var dbPath string
	for _, p := range cursorDBPaths() {
		if _, err := os.Stat(p); err == nil {
			dbPath = p
			break
		}
	}
	if dbPath == "" {
		return nil
	}
	if tenantID == "" {
		tenantID = "default"
	}
	return &CursorAdapter{
		pusher:   pusher,
		dbPath:   dbPath,
		done:     make(chan struct{}),
		tenantID: tenantID,
	}
}

// openProcDB opens (or creates) the cursor_processed table at procDBPath.
func openProcDB(procDBPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(procDBPath), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", procDBPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS cursor_processed (
		composer_id TEXT PRIMARY KEY
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Start begins the polling loop. Returns nil if the Cursor DB does not exist.
func (a *CursorAdapter) Start(ctx context.Context) error {
	if _, err := os.Stat(a.dbPath); os.IsNotExist(err) {
		return nil // Cursor not installed
	}
	go a.pollLoop(ctx)
	return nil
}

// Stop signals the polling loop to exit.
func (a *CursorAdapter) Stop() {
	close(a.done)
}

func (a *CursorAdapter) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run once immediately.
	a.sync(ctx)

	for {
		select {
		case <-ticker.C:
			a.sync(ctx)
		case <-ctx.Done():
			return
		case <-a.done:
			return
		}
	}
}

func (a *CursorAdapter) sync(ctx context.Context) {
	if a.procDB == nil {
		return
	}

	// Open once to collect all composer IDs, then close.
	composerIDs, err := a.listComposerIDs(ctx)
	if err != nil {
		return
	}

	for _, composerID := range composerIDs {
		a.syncComposer(ctx, composerID)
	}
}

// listComposerIDs opens the cursor DB, fetches all composerData keys, and closes.
func (a *CursorAdapter) listComposerIDs(ctx context.Context) ([]string, error) {
	db, err := sql.Open("sqlite", a.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx,
		"SELECT key FROM cursorDiskKV WHERE key LIKE 'composerData:%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		ids = append(ids, strings.TrimPrefix(key, "composerData:"))
	}
	return ids, nil
}

// syncComposer reads bubbles for composerID and pushes new ones.
// Opens a fresh DB connection per composer to avoid connection contention.
func (a *CursorAdapter) syncComposer(ctx context.Context, composerID string) {
	// Skip if already fully processed.
	var dummy string
	err := a.procDB.QueryRowContext(ctx,
		"SELECT composer_id FROM cursor_processed WHERE composer_id = ?", composerID).Scan(&dummy)
	if err == nil {
		return // already processed
	}

	msgs, err := a.fetchBubbles(ctx, composerID)
	if err != nil || len(msgs) == 0 {
		return
	}

	channelName := "cursor:" + composerID
	channelID, err := a.pusher.EnsureChannel(ctx, a.tenantID, "", channelName, c1push.PlatformCursor)
	if err != nil || channelID == "" {
		return
	}
	if err := a.pusher.AppendMessages(ctx, channelID, msgs); err != nil {
		return
	}

	// Mark as processed.
	_, _ = a.procDB.ExecContext(ctx,
		"INSERT OR IGNORE INTO cursor_processed (composer_id) VALUES (?)", composerID)
}

// fetchBubbles opens the cursor DB, reads all bubbles for composerID, and closes.
func (a *CursorAdapter) fetchBubbles(ctx context.Context, composerID string) ([]c1push.PushMessage, error) {
	db, err := sql.Open("sqlite", a.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx,
		"SELECT value FROM cursorDiskKV WHERE key LIKE ? ORDER BY key",
		"bubbleId:"+composerID+":%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []c1push.PushMessage
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			continue
		}
		msg := parseCursorBubble(value)
		if msg != nil {
			msgs = append(msgs, *msg)
		}
	}
	return msgs, nil
}

// cursorBubble is the JSON structure of a Cursor bubble entry.
type cursorBubble struct {
	Type   int    `json:"type"`   // 1=user, 2=assistant
	Text   string `json:"text"`
	UnixMs int64  `json:"unixMs"`
}

// parseCursorBubble parses a Cursor bubble JSON value into a PushMessage.
// Returns nil if the type is not 1 (user) or 2 (assistant).
func parseCursorBubble(value string) *c1push.PushMessage {
	var b cursorBubble
	if err := json.Unmarshal([]byte(value), &b); err != nil {
		return nil
	}
	var role string
	switch b.Type {
	case 1:
		role = "user"
	case 2:
		role = "assistant"
	default:
		return nil
	}
	return &c1push.PushMessage{
		SenderName: role,
		SenderType: role,
		Content:    b.Text,
	}
}
