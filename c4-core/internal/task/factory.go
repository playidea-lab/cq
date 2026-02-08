package task

import "fmt"

// Backend identifies which storage backend to use.
type Backend string

const (
	BackendMemory   Backend = "memory"   // In-memory Store (test/single-process)
	BackendSQLite   Backend = "sqlite"   // Alias for memory (same in-process store)
	BackendSupabase Backend = "supabase" // Supabase PostgREST
)

// StoreConfig holds the configuration needed to construct a TaskStore.
type StoreConfig struct {
	Backend   Backend         // which backend to use
	Supabase  *SupabaseConfig // required if Backend == BackendSupabase
}

// NewTaskStore creates a TaskStore based on the configured backend.
//
// Usage:
//
//	store, err := task.NewTaskStore(&task.StoreConfig{
//	    Backend: task.BackendSupabase,
//	    Supabase: &task.SupabaseConfig{
//	        URL:       "https://xxx.supabase.co",
//	        AnonKey:   "eyJ...",
//	        ProjectID: "my-project",
//	    },
//	})
func NewTaskStore(cfg *StoreConfig) (TaskStore, error) {
	if cfg == nil {
		return NewStore(), nil // default to in-memory
	}

	switch cfg.Backend {
	case BackendMemory, BackendSQLite, "":
		return NewStore(), nil

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
