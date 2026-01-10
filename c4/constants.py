"""C4 Constants - Centralized configuration values and magic numbers."""

# ============================================================================
# Task and Repair Constants
# ============================================================================

# Prefix for repair tasks created from blocked tasks
REPAIR_PREFIX = "REPAIR-"
REPAIR_PREFIX_LEN = len(REPAIR_PREFIX)

# Maximum depth of repair task nesting (REPAIR-REPAIR-{task})
MAX_REPAIR_DEPTH = 2

# ============================================================================
# Timeout Constants (in seconds)
# ============================================================================

# Default timeout for validation commands
DEFAULT_VALIDATION_TIMEOUT_SEC = 300

# Default timeout for supervisor review
DEFAULT_SUPERVISOR_TIMEOUT_SEC = 300

# SQLite connection busy timeout
SQLITE_BUSY_TIMEOUT_SEC = 30
SQLITE_BUSY_TIMEOUT_MS = SQLITE_BUSY_TIMEOUT_SEC * 1000

# Worker stale timeout (when to consider worker disconnected)
WORKER_STALE_TIMEOUT_SEC = 1800  # 30 minutes

# Task execution timeout
TASK_TIMEOUT_SEC = 1800  # 30 minutes

# Scope lock TTL (should match WORKER_STALE_TIMEOUT_SEC)
SCOPE_LOCK_TTL_SEC = 1800  # 30 minutes

# ============================================================================
# Retry Constants
# ============================================================================

# Maximum failures with same error signature before giving up
MAX_FAILURES_SAME_SIGNATURE = 3

# Maximum iterations per task before marking as blocked
MAX_ITERATIONS_PER_TASK = 7

# ============================================================================
# Default Intervals
# ============================================================================

# Polling interval for background loops
DEFAULT_POLL_INTERVAL_MS = 1000  # 1 second
