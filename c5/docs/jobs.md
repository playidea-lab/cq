# Jobs API

Submit, monitor, and manage jobs in the C5 Hub distributed queue.

## Endpoints

### POST /v1/jobs/submit

Submit a new job to the queue.

**Request:**
```json
{
  "name": "train-model",
  "command": "python train.py --epochs 100",
  "workdir": "/workspace/project",
  "env": {"CUDA_VISIBLE_DEVICES": "0"},
  "tags": ["ml", "training"],
  "requires_gpu": true,
  "priority": 5,
  "timeout_sec": 3600,
  "project_id": "my-project"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | no | Job name (default: "untitled") |
| command | string | **yes** | Shell command to execute |
| workdir | string | no | Working directory (default: ".") |
| env | object | no | Environment variables |
| tags | string[] | no | Tags for filtering |
| requires_gpu | bool | no | Whether the job needs a GPU |
| priority | int | no | Higher = higher priority (default: 0) |
| timeout_sec | int | no | Timeout in seconds |
| project_id | string | no | Project ID (overridden by API key scope) |

**Response** (201 Created):
```json
{
  "job_id": "j-abc123",
  "status": "QUEUED",
  "queue_position": 3
}
```

### GET /v1/jobs

List jobs with optional filters.

**Query Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| status | string | Filter by status: QUEUED, RUNNING, SUCCEEDED, FAILED, CANCELLED |
| project_id | string | Filter by project (overridden by API key scope) |
| limit | int | Max results (default: 50) |
| offset | int | Pagination offset |

**Response** (200 OK):
```json
[
  {
    "id": "j-abc123",
    "name": "train-model",
    "status": "RUNNING",
    "priority": 5,
    "command": "python train.py --epochs 100",
    "worker_id": "w-xyz",
    "created_at": "2026-02-16T10:00:00Z",
    "started_at": "2026-02-16T10:01:00Z"
  }
]
```

### GET /v1/jobs/{id}

Get a specific job by ID.

**Response** (200 OK):
```json
{
  "id": "j-abc123",
  "name": "train-model",
  "status": "SUCCEEDED",
  "priority": 5,
  "workdir": "/workspace/project",
  "command": "python train.py --epochs 100",
  "requires_gpu": true,
  "env": {"CUDA_VISIBLE_DEVICES": "0"},
  "tags": ["ml", "training"],
  "worker_id": "w-xyz",
  "created_at": "2026-02-16T10:00:00Z",
  "started_at": "2026-02-16T10:01:00Z",
  "finished_at": "2026-02-16T10:30:00Z",
  "exit_code": 0
}
```

### POST /v1/jobs/{id}/cancel

Cancel a queued or running job.

**Response** (200 OK):
```json
{
  "job_id": "j-abc123",
  "status": "CANCELLED"
}
```

### POST /v1/jobs/{id}/retry

Retry a failed or cancelled job. Creates a new job with the same parameters.

**Response** (200 OK):
```json
{
  "new_job_id": "j-def456",
  "status": "QUEUED",
  "original_job_id": "j-abc123"
}
```

### POST /v1/jobs/{id}/complete

Mark a job as completed (called by workers).

**Request:**
```json
{
  "status": "SUCCEEDED",
  "exit_code": 0
}
```

### GET /v1/jobs/{id}/logs

Get job log output.

**Query Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| offset | int | Line offset |
| limit | int | Max lines (default: 200) |

**Response** (200 OK):
```json
{
  "job_id": "j-abc123",
  "lines": ["Starting training...", "Epoch 1/100: loss=0.5"],
  "total_lines": 150,
  "offset": 0,
  "has_more": true
}
```

### GET /v1/jobs/{id}/summary

Get a compact job summary with latest metrics and log tail.

**Response** (200 OK):
```json
{
  "job_id": "j-abc123",
  "name": "train-model",
  "status": "RUNNING",
  "duration_seconds": 120.5,
  "metrics": {"loss": 0.3, "accuracy": 0.85},
  "log_tail": ["Epoch 99/100...", "Epoch 100/100..."]
}
```

### GET /v1/jobs/{id}/estimate

Get estimated duration and queue wait time.

**Response** (200 OK):
```json
{
  "estimated_duration_sec": 1800,
  "queue_wait_sec": 60,
  "estimated_start_time": "2026-02-16T10:02:00Z",
  "confidence": 0.75,
  "method": "historical"
}
```

## Job Statuses

| Status | Description |
|--------|-------------|
| QUEUED | Waiting in queue for a worker |
| RUNNING | Currently executing on a worker |
| SUCCEEDED | Completed successfully (exit_code = 0) |
| FAILED | Completed with errors (exit_code != 0) |
| CANCELLED | Cancelled by user |
