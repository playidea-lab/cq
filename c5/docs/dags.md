# DAGs API

Create and execute directed acyclic graph (DAG) workflows. DAGs define dependencies between jobs so they execute in the correct order.

## Endpoints

### POST /v1/dags

Create a new empty DAG.

**Request:**
```json
{
  "name": "ml-pipeline",
  "description": "Training and evaluation pipeline",
  "tags": ["ml", "pipeline"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | **yes** | DAG name |
| description | string | no | Human-readable description |
| tags | string[] | no | Tags for filtering |

**Response** (201 Created):
```json
{
  "id": "dag-abc123",
  "name": "ml-pipeline",
  "status": "pending",
  "nodes": [],
  "dependencies": [],
  "created_at": "2026-02-16T10:00:00Z"
}
```

### POST /v1/dags/from-yaml

Create a DAG from a YAML definition including nodes and dependencies.

**Request:**
```json
{
  "yaml_content": "name: ml-pipeline\nnodes:\n  - name: preprocess\n    command: python preprocess.py\n  - name: train\n    command: python train.py\ndependencies:\n  - source: preprocess\n    target: train"
}
```

**Response** (201 Created):
```json
{
  "id": "dag-abc123",
  "name": "ml-pipeline",
  "status": "pending",
  "nodes": [
    {"id": "n-001", "name": "preprocess", "command": "python preprocess.py", "status": "pending"},
    {"id": "n-002", "name": "train", "command": "python train.py", "status": "pending"}
  ],
  "dependencies": [
    {"source_id": "n-001", "target_id": "n-002", "dependency_type": "sequential"}
  ]
}
```

### GET /v1/dags

List all DAGs.

**Query Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| limit | int | Max results (default: 50) |
| offset | int | Pagination offset |

**Response** (200 OK):
```json
[
  {
    "id": "dag-abc123",
    "name": "ml-pipeline",
    "status": "completed",
    "created_at": "2026-02-16T10:00:00Z"
  }
]
```

### GET /v1/dags/{id}

Get DAG details including node statuses.

**Response** (200 OK):
```json
{
  "id": "dag-abc123",
  "name": "ml-pipeline",
  "status": "running",
  "nodes": [
    {"id": "n-001", "name": "preprocess", "status": "succeeded", "job_id": "j-001"},
    {"id": "n-002", "name": "train", "status": "running", "job_id": "j-002"}
  ],
  "dependencies": [
    {"source_id": "n-001", "target_id": "n-002"}
  ]
}
```

### POST /v1/dags/{id}/execute

Execute a DAG. Submits root nodes as jobs and orchestrates execution order based on dependencies.

**Request:**
```json
{
  "dry_run": false
}
```

**Response** (200 OK):
```json
{
  "dag_id": "dag-abc123",
  "status": "running",
  "node_order": ["preprocess", "train", "evaluate"],
  "validation": "ok"
}
```

### POST /v1/dags/{id}/nodes

Add a node to an existing DAG.

**Request:**
```json
{
  "name": "evaluate",
  "command": "python evaluate.py",
  "working_dir": "/workspace",
  "environment": {"MODEL_PATH": "/models/latest"},
  "gpu_count": 1,
  "max_retries": 2
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | **yes** | Node name |
| command | string | **yes** | Shell command to execute |
| working_dir | string | no | Working directory |
| environment | object | no | Environment variables |
| gpu_count | int | no | Required GPUs |
| max_retries | int | no | Max retry attempts |

**Response** (201 Created):
```json
{
  "node_id": "n-003",
  "name": "evaluate"
}
```

### POST /v1/dags/{id}/deps

Add a dependency between two nodes.

**Request:**
```json
{
  "source_id": "n-002",
  "target_id": "n-003",
  "dependency_type": "sequential"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| source_id | string | **yes** | Source node (must complete first) |
| target_id | string | **yes** | Target node (waits for source) |
| dependency_type | string | no | Type: sequential, data_dependency, conditional |

**Response** (200 OK):
```json
{
  "status": "added"
}
```

## DAG Statuses

| Status | Description |
|--------|-------------|
| pending | DAG created but not yet executed |
| running | DAG execution in progress |
| completed | All nodes completed successfully |
| failed | One or more nodes failed |
