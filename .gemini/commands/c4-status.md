# C4 Project Status

Show the current C4 project status with visual task graph progress.

## Instructions

### Step 1: Get Basic Status
Call `c4_status()` to get the current project status.

### Step 2: Get Task Details
Query the SQLite database to get all tasks with dependencies:
```bash
sqlite3 -json .c4/c4.db "SELECT task_id, status, task_json FROM c4_tasks ORDER BY task_id"
```

### Step 3: Display Status

#### 3.1 Basic Info
```
## C4 Status: [PROJECT_ID]
============================
**State:** [STATE] ([execution_mode])
```

#### 3.2 Overall Progress Bar
Calculate completion percentage and display:
```
### Overall Progress
████████████████░░░░░░░░░░░░░░░░░░░░░  XX% (done/total tasks)
```

#### 3.3 Phase Progress
Group tasks by phase (based on task ID ranges or prefix patterns):
- Parse task titles/IDs to identify phases
- Show each phase with its own progress bar

Format:
```
Phase N: [Name]           ████████████████████ 100% (X/Y)  ✅ Complete
├─ T-XXX [✅] Task title
├─ T-XXX [🔄] Task title (in progress)
├─ T-XXX [⏸] Task title ←── dependencies
```

Status icons:
- ✅ = done
- 🔄 = in_progress
- ⏸ = pending

#### 3.4 Queue Summary Table
```
### Queue
| Status | Count | Details |
|--------|-------|---------|
| Pending | X | T-XXX, T-XXX... |
| In Progress | X | T-XXX → worker-id |
| Done | X | ✓ |
```

#### 3.5 Workers Table
```
### Workers (N registered)
| Worker | State | Task |
|--------|-------|------|
| worker-id | **busy** | T-XXX |
| worker-id | idle | - |
```

#### 3.6 Metrics
```
### Metrics
- Tasks completed: **N**
- Events emitted: N
- Checkpoints passed: N
```

## Usage

```
/c4-status
```