# Workers API

Register and manage worker nodes that execute jobs from the queue.

## Endpoints

### POST /v1/workers/register

Register a new worker node.

**Request:**
```json
{
  "hostname": "gpu-server-1",
  "gpu_count": 2,
  "gpu_model": "A100",
  "total_vram_gb": 80,
  "free_vram_gb": 80,
  "tags": ["ml", "cuda"],
  "project_id": "my-project"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| hostname | string | **yes** | Worker hostname |
| gpu_count | int | no | Number of GPUs |
| gpu_model | string | no | GPU model name |
| total_vram_gb | float | no | Total VRAM in GB |
| free_vram_gb | float | no | Free VRAM in GB |
| tags | string[] | no | Worker capability tags |
| project_id | string | no | Project scope |

**Response** (201 Created):
```json
{
  "worker_id": "w-abc123"
}
```

### POST /v1/workers/heartbeat

Send a heartbeat to indicate the worker is still alive. Workers that miss heartbeats for 2 minutes are marked offline.

**Request:**
```json
{
  "worker_id": "w-abc123",
  "status": "online",
  "free_vram_gb": 40,
  "gpu_count": 2
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| worker_id | string | **yes** | Worker ID from registration |
| status | string | no | Worker status |
| free_vram_gb | float | no | Current free VRAM |
| gpu_count | int | no | Current GPU count |

**Response** (200 OK):
```json
{
  "acknowledged": true
}
```

### GET /v1/workers

List all registered workers.

**Response** (200 OK):
```json
[
  {
    "id": "w-abc123",
    "hostname": "gpu-server-1",
    "status": "online",
    "gpu_count": 2,
    "gpu_model": "A100",
    "total_vram_gb": 80,
    "free_vram_gb": 40,
    "last_heartbeat": "2026-02-16T10:05:00Z",
    "registered_at": "2026-02-16T09:00:00Z"
  }
]
```

## Lease Management

Workers acquire jobs through the lease system. A lease grants exclusive access to a job for a limited time.

### POST /v1/leases/acquire

Acquire a lease on the next available job.

**Request:**
```json
{
  "worker_id": "w-abc123",
  "free_vram_gb": 40
}
```

**Response** (200 OK) - Job available:
```json
{
  "job_id": "j-abc123",
  "lease_id": "l-xyz789",
  "job": {
    "id": "j-abc123",
    "name": "train-model",
    "command": "python train.py",
    "workdir": "/workspace"
  }
}
```

**Response** (200 OK) - No jobs:
```json
{
  "message": "no jobs available"
}
```

### POST /v1/leases/renew

Renew an active lease to prevent expiry. Leases expire after 5 minutes without renewal.

**Request:**
```json
{
  "lease_id": "l-xyz789",
  "worker_id": "w-abc123"
}
```

**Response** (200 OK):
```json
{
  "renewed": true,
  "new_expires_at": "2026-02-16T10:15:00Z"
}
```

## Worker Statuses

| Status | Description |
|--------|-------------|
| online | Worker is available and sending heartbeats |
| busy | Worker is executing a job |
| offline | Worker missed heartbeats (auto-set after 2 min) |
