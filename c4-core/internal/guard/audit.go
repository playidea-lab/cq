package guard

import "time"

// AuditEntry represents a single recorded access-control decision.
type AuditEntry struct {
	ID        int64
	Actor     string
	Tool      string
	Action    Action
	Reason    string
	CreatedAt time.Time
}
