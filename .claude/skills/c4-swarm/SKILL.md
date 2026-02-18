---
description: |
  Agent Teams-based parallel collaboration for C4 tasks. Spawns coordinator-led
  teams with direct member communication. Supports standard (implementation),
  review (read-only audit), and investigate (hypothesis competition) modes.
  Auto-maps C4 tasks to team tasks, domain-based agent selection, handoff tracking,
  auto-judge review spawning. Triggers: "팀 협업", "스웜", "병렬 팀 실행",
  "swarm mode", "spawn team", "parallel workers with communication",
  "team collaboration", "coordinate agents".
---

# C4 Swarm — Agent Teams Parallel Collaboration

**Team-based parallel execution with direct inter-member communication.** Maps C4 tasks to Agent Teams, with you as the coordinator.

## Usage

```
/c4-swarm                  # Auto: C4 task-based team composition
/c4-swarm 3                # Spawn 3 members
/c4-swarm --review         # Review-only team (read-only)
/c4-swarm --investigate    # Hypothesis competition mode
```

## vs /c4-run

| Item | `/c4-run` | `/c4-swarm` |
|------|-----------|-------------|
| Spawn | `Task(run_in_background)` | `Task(team_name=...)` |
| Tasks | C4 MCP only | Agent Teams TaskList + C4 MCP parallel |
| Communication | None (independent) | `SendMessage` (direct) |
| Lifetime | 1 task → exit | Until team disbands |
| Coordinator | None | Main session = team lead |
| Monitoring | `tail -f` | Auto message receive |

**Simple parallel → `/c4-run`, team collaboration → `/c4-swarm`.**

---

## Instructions

### 0. Parse Arguments

```python
args = "$ARGUMENTS".strip()

review_mode = "--review" in args
investigate_mode = "--investigate" in args

# Determine member count
if review_mode:
    member_count = 3  # Security / Performance / Test Coverage
elif investigate_mode:
    member_count = 3  # Default 3
elif args and args.isdigit():
    member_count = min(int(args), 5)
else:
    member_count = None  # Auto-decide in Step 1
```

### 1. Check C4 Status + Auto-determine Team Size

```python
status = mcp__c4__c4_status()
```

Branch by state:
- **INIT**: "먼저 `/c4-plan`으로 계획을 수립하세요." → exit
- **CHECKPOINT**: "Checkpoint 리뷰 대기 중입니다. `/c4-checkpoint` 실행 필요." → exit
- **COMPLETE**: "프로젝트가 완료되었습니다." → exit
- **PLAN/HALTED**: `mcp__c4__c4_start()` → transition to EXECUTE then continue
- **EXECUTE**: proceed

```python
if member_count is None:
    parallelism = status["parallelism"]
    member_count = min(parallelism["recommended"], 5)

print(f"""
C4 Swarm Analysis:
  State: {status["status"]}
  Ready: {status["parallelism"]["ready_now"]} tasks
  Team Size: {member_count} members
  Mode: {"review" if review_mode else "investigate" if investigate_mode else "standard"}
""")
```

### 2. Create Agent Team

```python
import time

team_name = f"c4-{int(time.time())}"

TeamCreate(team_name=team_name, description=f"C4 Swarm — {member_count} members")
```

### 3. Map C4 Tasks → Agent Teams TaskCreate

**Non-review/investigate modes**, map C4 pending tasks to Agent Teams tasks:

```python
# After confirming pending tasks from c4_status
# Register each as Agent Teams TaskCreate

for task in pending_tasks:
    TaskCreate(
        subject=f"[{task['id']}] {task['title']}",
        description=f"""C4 task {task['id']} implementation.

DoD: {task['dod']}
Files: {task.get('files', 'N/A')}

On completion:
1. Implement + test
2. c4_submit(task_id="{task['id']}", ...)
3. TaskUpdate(status="completed")
4. SendMessage to coordinator with report
""",
        activeForm=f"Implementing {task['id']}"
    )
```

**--review mode**: Analyze review targets (files/commits) and create review tasks.
**--investigate mode**: Create tasks per hypothesis for bug/issue investigation.

### 4. Spawn Members

Assign role and domain to each member.

#### Standard Mode Member Prompt

```python
MEMBER_PROMPT = """You are "{member_name}", a member of team "{team_name}".

## Your Role
{role_description}

## Workflow
1. TaskList() → select lowest ID among unassigned/unblocked
2. TaskUpdate(taskId, status="in_progress", owner="{member_name}")
3. Perform implementation
   - Use C4 MCP tools: c4_find_symbol, c4_read_file, c4_search_for_pattern, etc.
   - File modifications: Edit/Write tools
   - Validation: uv run python -m py_compile (Python) / go build (Go)
4. git commit
5. **Write handoff** (JSON format) then submit:
   c4_submit(task_id, worker_id="{member_name}", commit_sha, validation_results,
     handoff=json.dumps({"summary": "...", "files_changed": [...],
       "discoveries": [...], "concerns": [...], "rationale": "..."}))
6. TaskUpdate(taskId, status="completed")
7. SendMessage(type="message", recipient="coordinator",
     content="[task_id] completed. Handoff: [key discovery summary]", summary="Task done + handoff")
8. TaskList() → if next task exists goto 2, else wait

## Handoff Writing Rules (CRITICAL)
handoff 파라미터는 JSON 문자열로 전달 (autoRecordKnowledge가 파싱):
```json
{
  "summary": "구현 요약",
  "files_changed": ["path/to/file.go"],
  "discoveries": ["발견사항: 의존성, 사이드이펙트, 숨겨진 복잡성"],
  "concerns": ["우려사항: 버그, 성능, 미완성 부분"],
  "rationale": "설계 결정 이유"
}
```

## Communication
- Need to coordinate with other members: SendMessage(type="message", recipient="member_name", ...)
- Report to coordinator: SendMessage(type="message", recipient="coordinator", ...)
- If blocked: notify coordinator via SendMessage

## Important
- Process one task at a time
- After task completion, always check next task via TaskList
- If shutdown_request received, respond with approve
"""
```

#### Review Mode Member Prompt

```python
REVIEW_MEMBER_PROMPT = """You are "{member_name}", a {review_focus} reviewer on team "{team_name}".

## Your Focus: {review_focus}
{review_description}

## Workflow
1. TaskList() → select your review task
2. TaskUpdate(taskId, status="in_progress", owner="{member_name}")
3. Read-only code analysis — use Read, Grep, Glob, c4_find_symbol, etc.
4. Report review results to coordinator via SendMessage:
   - Issues found (severity: critical/warning/info)
   - Specific file:line locations
   - Improvement suggestions
5. TaskUpdate(taskId, status="completed")
6. If shutdown_request received, respond with approve

## CRITICAL: No File Modifications
Do not use Edit/Write tools. This is read-only review.
"""
```

#### Investigate Mode Member Prompt

```python
INVESTIGATE_MEMBER_PROMPT = """You are "{member_name}", investigating hypothesis "{hypothesis}" on team "{team_name}".

## Your Hypothesis
{hypothesis_description}

## Workflow
1. TaskList() → select your hypothesis task
2. TaskUpdate(taskId, status="in_progress", owner="{member_name}")
3. Investigate:
   - Code analysis (Read, Grep, c4_find_symbol, etc.)
   - Check logs/state
   - Run experiments (if needed)
4. Discuss with other members:
   - Share findings via SendMessage
   - Present counter-evidence
5. After reaching conclusion, report to coordinator:
   - Hypothesis support/refute status
   - Evidence summary
   - Recommended actions
6. TaskUpdate(taskId, status="completed")
7. If shutdown_request received, respond with approve
"""
```

#### Execute Member Spawning

```python
members = []

if review_mode:
    review_roles = [
        ("security-reviewer", "Security", "보안 취약점, 인증/권한, 입력 검증, 인젝션 공격 벡터를 중심으로 리뷰"),
        ("perf-reviewer", "Performance", "성능 병목, N+1 쿼리, 불필요한 할당, 캐싱 기회를 중심으로 리뷰"),
        ("test-reviewer", "Test Coverage", "테스트 커버리지, 엣지 케이스, 회귀 위험, 테스트 품질을 중심으로 리뷰"),
    ]
    for name, focus, desc in review_roles:
        Task(
            subagent_type="general-purpose",
            name=name,
            team_name=team_name,
            description=f"{focus} Reviewer",
            prompt=REVIEW_MEMBER_PROMPT.format(
                member_name=name, team_name=team_name,
                review_focus=focus, review_description=desc
            ),
            mode="plan",  # Read-only
        )
        members.append(name)

elif investigate_mode:
    # Identify investigation targets from C4 state, distribute hypotheses
    hypotheses = [
        ("investigator-1", "가설 A", "첫 번째 가설 설명"),
        ("investigator-2", "가설 B", "두 번째 가설 설명"),
        ("investigator-3", "가설 C", "세 번째 가설 설명"),
    ]
    for name, hyp, desc in hypotheses[:member_count]:
        Task(
            subagent_type="general-purpose",
            name=name,
            team_name=team_name,
            description=f"Investigator: {hyp}",
            prompt=INVESTIGATE_MEMBER_PROMPT.format(
                member_name=name, team_name=team_name,
                hypothesis=hyp, hypothesis_description=desc
            ),
        )
        members.append(name)

else:
    # Standard mode: auto-map domain-specialized agents
    DOMAIN_AGENT_MAP = {
        "security": "security-auditor",
        "frontend": "frontend-developer",
        "backend": "backend-architect",
        "database": "database-optimizer",
        "devops": "deployment-engineer",
        "ml": "ml-engineer",
        "api": "backend-architect",
        "testing": "test-automator",
        "performance": "performance-engineer",
        "go": "golang-pro",
        "python": "python-pro",
        "infra": "cloud-architect",
    }

    # Check task domains → select specialized agents
    task_domains = {}  # task_id → domain
    for task in pending_tasks:
        domain = task.get("domain", "").lower()
        task_domains[task["id"]] = domain

    # Spawn members (auto-select agent_type by domain)
    for i in range(member_count):
        name = f"worker-{i+1}"
        # Determine agent_type by likely domain for this worker
        agent_type = "general-purpose"  # default
        if i < len(pending_tasks):
            domain = task_domains.get(pending_tasks[i]["id"], "")
            agent_type = DOMAIN_AGENT_MAP.get(domain, "general-purpose")

        Task(
            subagent_type=agent_type,
            name=name,
            team_name=team_name,
            description=f"C4 Worker {i+1}/{member_count} ({agent_type})",
            prompt=MEMBER_PROMPT.format(
                member_name=name, team_name=team_name,
                role_description=f"팀의 {i+1}번째 구현 담당 (전문: {agent_type}). TaskList에서 미할당 태스크를 선택하여 구현합니다."
            ),
            mode="bypassPermissions",
        )
        members.append(name)
```

Spawn all members **simultaneously (in parallel)**. Send multiple Task calls in one message.

### 5. Coordinator Role (You = Team Lead)

After team creation, perform as coordinator:

```
C4 Swarm started! (Team: {team_name})

Members: {members}
Mode: {mode}

Member messages will be auto-received.
You can instruct members via SendMessage as needed.

Shift+Tab to toggle delegate mode.
```

**React to auto-received member messages**:
- Task completion report → **check handoff** + guide next task
- Block report → provide solution or delegate to another member
- Questions → answer
- Review results → synthesize

#### 5.1 Auto-Judge: Automatic Review Spawning (CRITICAL)

When a worker submits a T- task, c4_submit response includes `pending_review` field.
In this case, **immediately spawn a review agent**:

```python
# When receiving worker completion message
if submit_result.get("pending_review"):
    review_task_id = submit_result["pending_review"]
    review_name = f"reviewer-{review_task_id}"
    Task(
        subagent_type="code-reviewer",
        name=review_name,
        team_name=team_name,
        description=f"Auto-review {review_task_id}",
        prompt=f"""You are "{review_name}", an auto-judge reviewer on team "{team_name}".

Review task {review_task_id}. This is the review for a completed implementation.

Workflow:
1. c4_get_task(worker_id="{review_name}") — this will assign {review_task_id}
2. Read the implementation (review_context has parent task info, commit SHA, files)
3. Read the implementer's handoff (discoveries, concerns, feedback)
4. Review against DoD, Soul principles, and code quality
5. If APPROVED: c4_checkpoint(decision="APPROVE", notes="...")
6. If NEEDS CHANGES: c4_request_changes(review_task_id="{review_task_id}", comments="...", required_changes=[...])
7. SendMessage to coordinator with verdict
8. TaskUpdate(status="completed")
""",
        mode="plan",  # Read-only
    )
```

#### 5.2 Recursive Sub-planners (Complex Task Breakdown)

If a task **has subtasks or is multi-domain**, spawn a **sub-planner** instead of worker:

```python
SUB_PLANNER_PROMPT = """You are "{member_name}", a sub-planner on team "{team_name}".

## Your Scope
{scope_description}

## Workflow
1. Analyze assigned task and break into 2-4 subtasks
2. Register each subtask via c4_add_todo (set parent task dependency)
3. Spawn workers per subtask (Task tool, team_name="{team_name}")
4. Receive completion reports from workers and synthesize handoffs
5. When all subtasks complete, report synthesized handoff to coordinator
6. Send shutdown_request to workers

## Breakdown Criteria
- Separate by low file dependency units
- Each subtask must be independently verifiable
- Set dependencies if subtasks have ordering requirements
"""

# Sub-planner spawn condition: task scope has 3+ files, or DoD has 5+ checkboxes
if task_is_complex(task):
    Task(
        subagent_type="general-purpose",
        name=f"planner-{task['id']}",
        team_name=team_name,
        description=f"Sub-planner for {task['id']}",
        prompt=SUB_PLANNER_PROMPT.format(
            member_name=f"planner-{task['id']}",
            team_name=team_name,
            scope_description=f"Task {task['id']}: {task['title']}\nDoD: {task['dod']}"
        ),
        mode="bypassPermissions",
    )
```

### 6. Disband Team

When all tasks complete:

```python
# 1. Send shutdown request to each member
for member in members:
    SendMessage(type="shutdown_request", recipient=member, content="모든 태스크 완료. 수고하셨습니다!")

# 2. After members approve shutdown
TeamDelete()

print(f"""
C4 Swarm complete! (Team: {team_name})
  Tasks processed: N
  Members: {len(members)}
  Mode: {mode}
""")
```

---

## Expected Flows

### Standard Mode
```
/c4-swarm 3
→ Check C4 state: EXECUTE, 5 tasks ready
→ TeamCreate("c4-1707500000")
→ 5 C4 tasks → Agent Teams TaskCreate
→ Spawn worker-1, worker-2, worker-3
→ Each member: TaskList → claim → implement → submit → next task
→ Members coordinate via SendMessage as needed
→ All tasks complete → shutdown → TeamDelete
```

### Review Mode
```
/c4-swarm --review
→ Check recent changed files from C4 state
→ TeamCreate + spawn 3 reviewers (Security, Performance, Test)
→ Each reviewer analyzes code read-only
→ Report review results to coordinator via SendMessage
→ Coordinator synthesizes → c4_request_changes or approve
→ shutdown → TeamDelete
```

### Investigate Mode
```
/c4-swarm --investigate
→ Formulate 3 hypotheses for bug/issue
→ TeamCreate + spawn 3 investigators
→ Each independently verifies hypothesis
→ Share evidence + discuss via SendMessage
→ Converge then report conclusion to coordinator
→ shutdown → TeamDelete
```

---

## Constraints

| Constraint | Description |
|------------|-------------|
| Max Members | 5 (stability first) |
| Review Mode | plan mode (read-only) |
| Accept Edits | Required in Standard mode (`Shift+Tab`) |
| delegate mode | Recommended for coordinator (`Shift+Tab`) |

## Related Skills

- `/c4-run` — Independent Worker parallel execution (no communication, fire-and-forget)
- `/c4-status` — Check C4 project state
- `/c4-checkpoint` — Checkpoint review
