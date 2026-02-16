# Artifacts API

Upload, download, and manage job artifacts. Artifacts are files produced by jobs that can be deployed to edge devices or downloaded for analysis.

## Endpoints

### POST /v1/storage/upload

Upload an artifact file directly.

**Request:** multipart/form-data
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| file | file | **yes** | File to upload |
| path | string | **yes** | Storage path (e.g., "jobs/j-abc123/model.onnx") |

**Response** (200 OK):
```json
{
  "path": "jobs/j-abc123/model.onnx",
  "size_bytes": 1048576,
  "url": "/storage/jobs/j-abc123/model.onnx"
}
```

### POST /v1/storage/presigned-url

Generate a presigned URL for direct upload or download.

**Request:**
```json
{
  "path": "jobs/j-abc123/model.onnx",
  "method": "PUT",
  "ttl_seconds": 3600,
  "content_type": "application/octet-stream"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| path | string | **yes** | Storage path |
| method | string | no | "GET" or "PUT" (default: "GET") |
| ttl_seconds | int | no | URL expiration time |
| content_type | string | no | Content type for PUT uploads |

**Response** (200 OK):
```json
{
  "url": "https://storage.example.com/signed-url...",
  "expires_at": "2026-02-16T11:00:00Z"
}
```

### GET /v1/artifacts/{job_id}

List all artifacts for a job.

**Response** (200 OK):
```json
[
  {
    "id": "a-001",
    "job_id": "j-abc123",
    "path": "jobs/j-abc123/model.onnx",
    "content_hash": "sha256:abc123...",
    "size_bytes": 1048576,
    "confirmed": true,
    "created_at": "2026-02-16T10:30:00Z"
  }
]
```

### POST /v1/artifacts/{job_id}/confirm

Confirm that an artifact has been successfully uploaded.

**Request:**
```json
{
  "path": "jobs/j-abc123/model.onnx",
  "content_hash": "sha256:abc123def456...",
  "size_bytes": 1048576
}
```

**Response** (200 OK):
```json
{
  "artifact_id": "a-001",
  "confirmed": true
}
```

### GET /v1/artifacts/{job_id}/url/{filename}

Get a download URL for a specific artifact.

**Response** (200 OK):
```json
{
  "url": "https://storage.example.com/signed-url..."
}
```
