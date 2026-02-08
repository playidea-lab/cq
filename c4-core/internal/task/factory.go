package task

import (
	"database/sql"
	"fmt"
)

// Backend identifies which storage backend to use.
type Backend string

const (
	BackendMemory   Backend = "memory"   // In-memory Store (test/single-process)
	BackendSQLite   Backend = "sqlite"   // SQLite-backed store (.c4/c4.db compatible)
	BackendSupabase Backend = "supabase" // Supabase PostgREST
)

// SQLiteConfig holds SQLite-specific configuration.
type SQLiteConfig struct {
	DB        *sql.DB // Open database handle
	ProjectID string  // C4 project ID for table partitioning
}

// StoreConfig holds the configuration needed to construct a TaskStore.
type StoreConfig struct {
	Backend  Backend         // which backend to use
	SQLite   *SQLiteConfig   // required if Backend == BackendSQLite
	Supabase *SupabaseConfig // required if Backend == BackendSupabase
}

// NewTaskStore creates a TaskStore based on the configured backend.
//
// Usage:
//
//	store, err := task.NewTaskStore(&task.StoreConfig{
//	    Backend: task.BackendSQLite,
//	    SQLite: &task.SQLiteConfig{
//	        DB:        db,
//	        ProjectID: "my-project",
//	    },
//	})
func NewTaskStore(cfg *StoreConfig) (TaskStore, error) {
	if cfg == nil {
		return NewStore(), nil // default to in-memory
	}

	switch cfg.Backend {
	case BackendMemory, "":
		return NewStore(), nil

	case BackendSQLite:
		if cfg.SQLite == nil {
			return nil, fmt.Errorf("sqlite config is required for backend %q", cfg.Backend)
		}
		if cfg.SQLite.DB == nil {
			return nil, fmt.Errorf("sqlite DB handle is required")
		}
		return NewSQLiteTaskStore(cfg.SQLite.DB, cfg.SQLite.ProjectID)

	case BackendSupabase:
		if cfg.Supabase == nil {
			return nil, fmt.Errorf("supabase config is required for backend %q", cfg.Backend)
		}
		if cfg.Supabase.URL == "" || cfg.Supabase.AnonKey == "" {
			return nil, fmt.Errorf("supabase URL and AnonKey are required")
		}
		return NewSupabaseTaskStore(cfg.Supabase), nil

	default:
		return nil, fmt.Errorf("unknown task store backend: %q", cfg.Backend)
	}
}
