# Edges API

Register and manage edge devices for model deployment. Edge devices receive artifacts from completed jobs.

## Endpoints

### POST /v1/edges/register

Register a new edge device.

**Request:**
```json
{
  "name": "jetson-nano-01",
  "tags": ["onnx", "arm64"],
  "arch": "arm64",
  "runtime": "onnxruntime",
  "storage_gb": 32,
  "metadata": {"location": "factory-floor-1"}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | **yes** | Device name |
| tags | string[] | no | Capability tags for filtering |
| arch | string | no | Architecture (arm64, x86_64) |
| runtime | string | no | Inference runtime |
| storage_gb | float | no | Available storage in GB |
| metadata | object | no | Additional key-value metadata |

**Response** (201 Created):
```json
{
  "edge_id": "e-abc123"
}
```

### POST /v1/edges/heartbeat

Send a heartbeat from an edge device. Edges missing heartbeats for 5 minutes are marked offline.

**Request:**
```json
{
  "edge_id": "e-abc123",
  "status": "online"
}
```

**Response** (200 OK):
```json
{
  "acknowledged": true
}
```

### GET /v1/edges

List all registered edge devices.

**Response** (200 OK):
```json
[
  {
    "id": "e-abc123",
    "name": "jetson-nano-01",
    "status": "online",
    "tags": ["onnx", "arm64"],
    "arch": "arm64",
    "runtime": "onnxruntime",
    "storage_gb": 32,
    "last_seen": "2026-02-16T10:05:00Z"
  }
]
```

### GET /v1/edges/{id}

Get details for a specific edge device.

**Response** (200 OK):
```json
{
  "id": "e-abc123",
  "name": "jetson-nano-01",
  "status": "online",
  "tags": ["onnx", "arm64"],
  "arch": "arm64",
  "runtime": "onnxruntime",
  "storage_gb": 32,
  "metadata": {"location": "factory-floor-1"},
  "last_seen": "2026-02-16T10:05:00Z"
}
```

## Deployment

### POST /v1/deploy/rules

Create a deployment rule for automatic artifact deployment.

**Request:**
```json
{
  "name": "auto-deploy-onnx",
  "trigger": "job.succeeded",
  "edge_filter": "onnx",
  "artifact_pattern": "*.onnx",
  "post_command": "systemctl restart inference"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | no | Rule name |
| trigger | string | **yes** | Event trigger (e.g., job.succeeded) |
| edge_filter | string | **yes** | Tag filter for target edges |
| artifact_pattern | string | **yes** | Glob pattern for artifacts |
| post_command | string | no | Command to run after deployment |

**Response** (201 Created):
```json
{
  "rule_id": "r-abc123"
}
```

### GET /v1/deploy/rules

List all deployment rules.

**Response** (200 OK):
```json
[
  {
    "id": "r-abc123",
    "name": "auto-deploy-onnx",
    "trigger": "job.succeeded",
    "edge_filter": "onnx",
    "artifact_pattern": "*.onnx",
    "enabled": true
  }
]
```

### POST /v1/deploy/trigger

Manually trigger a deployment to specific edges.

**Request:**
```json
{
  "job_id": "j-abc123",
  "edge_ids": ["e-001", "e-002"],
  "artifact_pattern": "*.onnx",
  "post_command": "systemctl restart inference"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| job_id | string | **yes** | Source job ID |
| edge_ids | string[] | no | Target edge IDs (if empty, uses edge_filter) |
| edge_filter | string | no | Tag filter for target edges |
| artifact_pattern | string | no | Glob pattern for artifacts |
| post_command | string | no | Post-deployment command |

**Response** (201 Created):
```json
{
  "deploy_id": "d-abc123",
  "status": "pending",
  "target_count": 2
}
```

### GET /v1/deploy/{id}

Get deployment status.

**Response** (200 OK):
```json
{
  "id": "d-abc123",
  "job_id": "j-abc123",
  "status": "completed",
  "targets": [
    {"edge_id": "e-001", "edge_name": "jetson-01", "status": "succeeded"},
    {"edge_id": "e-002", "edge_name": "jetson-02", "status": "succeeded"}
  ],
  "created_at": "2026-02-16T10:00:00Z",
  "finished_at": "2026-02-16T10:05:00Z"
}
```

## Edge Statuses

| Status | Description |
|--------|-------------|
| online | Device is connected and sending heartbeats |
| offline | Device missed heartbeats (auto-set after 5 min) |

## Deploy Statuses

| Status | Description |
|--------|-------------|
| pending | Deployment created, not yet started |
| deploying | Artifacts being transferred |
| completed | All targets succeeded |
| failed | All targets failed |
| partial | Some targets succeeded, some failed |
