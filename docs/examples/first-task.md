# Your First Task with CQ

A complete walkthrough of creating and completing your first CQ task — from zero to a working commit.

---

## What You'll Build

In this example, you'll add a simple health-check endpoint to an existing Go API. The workflow covers:

1. Initializing CQ in your project
2. Creating a task with a clear definition of done
3. Running a worker to implement it
4. Submitting the result

Total time: ~5 minutes.

---

## Prerequisites

- CQ installed (`cq --version` prints a version number)
- A Go project open in Claude Code
- Git repository initialized

---

## Step 1: Check CQ Status

Open Claude Code in your project directory. First, check whether CQ is already initialized:

```
/c4-status
```

**Expected output if not initialized:**

```
CQ not initialized. Run /c4-init to set up.
```

**Expected output if already initialized:**

```
## CQ Status
- Project: my-api
- State: IDLE
- Queue: 0 pending | 0 in_progress | 0 done
```

If not initialized, run `/c4-init` and follow the prompts.

---

## Step 2: Create the Task

Use `/c4-quick` for small, well-defined tasks (1–5 files, clear scope):

```
/c4-quick "add GET /health endpoint returning {status: ok, version: string}"
```

CQ creates a task and immediately assigns it to you:

```
Task created: T-001
Title: add GET /health endpoint returning {status: ok, version: string}
Scope: auto-detected (Go backend)
Status: in_progress (claimed by current session)
```

**What happens internally:**

- CQ creates task T-001 in `.c4/tasks.db`
- The task is auto-claimed — no separate `/c4-claim` needed with `/c4-quick`
- CQ scans the project to detect Go files and sets `domain: go`

---

## Step 3: Implement the Endpoint

Now implement the feature. CQ doesn't write the code for you in quick mode — you do. Find or create the appropriate handler file:

```go
// internal/api/health.go
package api

import (
    "encoding/json"
    "net/http"
)

var version = "1.0.0"

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status":  "ok",
        "version": version,
    })
}
```

Register the route in your router:

```go
// internal/api/router.go
mux.HandleFunc("GET /health", handleHealth)
```

---

## Step 4: Validate

Before submitting, run validation:

```
/c4-validate
```

**Expected output:**

```
Running validations...
  go-build:  PASS  (cd . && go build ./...)
  go-vet:    PASS  (cd . && go vet ./...)

All validations passed.
```

If validation fails, fix the errors before continuing. CQ will refuse to submit a task with failed validation.

**Common failure: build error**

```
go-build: FAIL
  ./internal/api/health.go:8:2: undefined: json
  → Add "encoding/json" to imports
```

Fix the issue and re-run `/c4-validate`.

---

## Step 5: Submit

Once validation passes:

```
/c4-submit
```

**Expected output:**

```
Submitting T-001...
  Commit: abc1234 "feat(api): add GET /health endpoint"
  Validation: all passed
  Status: done

Task T-001 completed.
```

CQ automatically:

1. Runs a final validation check
2. Creates a git commit with the task description
3. Marks the task as `done` in the queue

---

## Step 6: Verify

Check the final state:

```
/c4-status
```

```
## CQ Status
- Project: my-api
- State: IDLE
- Queue: 0 pending | 0 in_progress | 1 done

Done:
  T-001  add GET /health endpoint        abc1234
```

Test the endpoint manually:

```bash
go run . &
curl http://localhost:8080/health
# {"status":"ok","version":"1.0.0"}
```

---

## What You Learned

| Concept | Command | When to Use |
|---------|---------|-------------|
| Quick task | `/c4-quick "description"` | 1–5 files, requirements are clear |
| Validate | `/c4-validate` | Before every submit |
| Submit | `/c4-submit` | After validation passes |
| Check state | `/c4-status` | Anytime |

---

## Next Steps

- **Bug fix scenario**: [Bug Fix with /c4-quick](bug-fix.md)
- **Larger feature**: [Feature Planning with /pi and /c4-plan](feature-planning.md)
- **Full command reference**: [Usage Guide](../usage-guide.md)
