# C4 Swarm — Agent Teams 기반 병렬 협업

**팀원 간 직접 통신이 가능한 협업형 병렬 실행.** C4 태스크를 Agent Teams로 매핑하여, coordinator(당신)가 팀을 지휘합니다.

## Usage

```
/c4-swarm                  # 자동: C4 태스크 기반 팀 구성
/c4-swarm 3                # 3명 팀원 스폰
/c4-swarm --review         # 리뷰 전용 팀 (읽기 전용)
/c4-swarm --investigate    # 가설 경쟁 모드
```

## /c4-run과의 차이

| 항목 | `/c4-run` | `/c4-swarm` |
|------|-----------|-------------|
| 스폰 | `Task(run_in_background)` | `Task(team_name=...)` |
| 태스크 | C4 MCP만 | Agent Teams TaskList + C4 MCP 병행 |
| 통신 | 없음 (독립) | `SendMessage` (팀원 간 직접) |
| 수명 | 1 task → exit | 팀 해산까지 유지 |
| coordinator | 없음 | 메인 세션 = 팀장 |
| 모니터링 | `tail -f` | 메시지 자동 수신 |

**간단한 병렬 실행은 `/c4-run`, 팀원 간 협업이 필요하면 `/c4-swarm`.**

---

## Instructions

### 0. 인자 파싱

```python
args = "$ARGUMENTS".strip()

review_mode = "--review" in args
investigate_mode = "--investigate" in args

# 팀원 수 결정
if review_mode:
    member_count = 3  # Security / Performance / Test Coverage
elif investigate_mode:
    member_count = 3  # 기본 3명
elif args and args.isdigit():
    member_count = min(int(args), 5)
else:
    member_count = None  # 자동 결정 (Step 1에서)
```

### 1. C4 상태 확인 + 팀원 수 자동 결정

```python
status = mcp__c4__c4_status()
```

상태에 따른 분기:
- **INIT**: "먼저 `/c4-plan`으로 계획을 수립하세요." → 종료
- **CHECKPOINT**: "Checkpoint 리뷰 대기 중입니다. `/c4-checkpoint` 실행 필요." → 종료
- **COMPLETE**: "프로젝트가 완료되었습니다." → 종료
- **PLAN/HALTED**: `mcp__c4__c4_start()` → EXECUTE 전환 후 계속
- **EXECUTE**: 계속 진행

```python
if member_count is None:
    parallelism = status["parallelism"]
    member_count = min(parallelism["recommended"], 5)

print(f"""
C4 Swarm 분석:
  상태: {status["status"]}
  실행 가능: {status["parallelism"]["ready_now"]}개 태스크
  팀원 수: {member_count}명
  모드: {"review" if review_mode else "investigate" if investigate_mode else "standard"}
""")
```

### 2. Agent Teams 팀 생성

```python
import time

team_name = f"c4-{int(time.time())}"

TeamCreate(team_name=team_name, description=f"C4 Swarm — {member_count} members")
```

### 3. C4 태스크 → Agent Teams TaskCreate 매핑

**review/investigate 모드가 아닌 경우**, C4 pending 태스크를 Agent Teams 태스크로 매핑:

```python
# c4_status에서 pending 태스크 목록 확인 후
# 각 태스크를 Agent Teams TaskCreate로 등록

for task in pending_tasks:
    TaskCreate(
        subject=f"[{task['id']}] {task['title']}",
        description=f"""C4 태스크 {task['id']} 구현.

DoD: {task['dod']}
Files: {task.get('files', 'N/A')}

완료 시:
1. 구현 + 테스트
2. c4_submit(task_id="{task['id']}", ...)
3. TaskUpdate(status="completed")
4. SendMessage로 coordinator에 보고
""",
        activeForm=f"Implementing {task['id']}"
    )
```

**--review 모드**: 리뷰 대상 파일/커밋을 분석해서 리뷰 태스크 생성.
**--investigate 모드**: 조사 대상 버그/이슈를 가설별로 태스크 생성.

### 4. 팀원 스폰

각 팀원에게 역할과 영역을 지정하여 스폰합니다.

#### Standard 모드 팀원 프롬프트

```python
MEMBER_PROMPT = """You are "{member_name}", a member of team "{team_name}".

## Your Role
{role_description}

## Workflow
1. TaskList() → 미할당/비차단 태스크 중 가장 낮은 ID 선택
2. TaskUpdate(taskId, status="in_progress", owner="{member_name}")
3. 구현 작업 수행
   - C4 MCP 도구 사용: c4_find_symbol, c4_read_file, c4_search_for_pattern 등
   - 파일 수정: Edit/Write 도구
   - 검증: uv run python -m py_compile (Python) / go build (Go)
4. git commit
5. c4_submit(task_id, worker_id="{member_name}", commit_sha, validation_results)
6. TaskUpdate(taskId, status="completed")
7. SendMessage(type="message", recipient="coordinator", content="[task_id] 완료", summary="Task done")
8. TaskList() → 다음 태스크가 있으면 2번으로, 없으면 대기

## Communication
- 다른 팀원과 협의 필요 시: SendMessage(type="message", recipient="팀원이름", ...)
- coordinator에게 보고: SendMessage(type="message", recipient="coordinator", ...)
- 차단(blocked) 시: SendMessage로 coordinator에 알림

## Important
- 한 번에 하나의 태스크만 처리
- 태스크 완료 후 반드시 TaskList로 다음 태스크 확인
- shutdown_request를 받으면 approve로 응답
"""
```

#### Review 모드 팀원 프롬프트

```python
REVIEW_MEMBER_PROMPT = """You are "{member_name}", a {review_focus} reviewer on team "{team_name}".

## Your Focus: {review_focus}
{review_description}

## Workflow
1. TaskList() → 자신의 리뷰 태스크 선택
2. TaskUpdate(taskId, status="in_progress", owner="{member_name}")
3. 코드 읽기 전용 — Read, Grep, Glob, c4_find_symbol 등으로 분석
4. 리뷰 결과를 SendMessage로 coordinator에 보고:
   - 발견한 이슈 (심각도: critical/warning/info)
   - 구체적 파일:라인 위치
   - 개선 제안
5. TaskUpdate(taskId, status="completed")
6. shutdown_request를 받으면 approve로 응답

## CRITICAL: 파일 수정 금지
Edit/Write 도구를 사용하지 마세요. 읽기 전용 리뷰입니다.
"""
```

#### Investigate 모드 팀원 프롬프트

```python
INVESTIGATE_MEMBER_PROMPT = """You are "{member_name}", investigating hypothesis "{hypothesis}" on team "{team_name}".

## Your Hypothesis
{hypothesis_description}

## Workflow
1. TaskList() → 자신의 가설 태스크 선택
2. TaskUpdate(taskId, status="in_progress", owner="{member_name}")
3. 조사:
   - 코드 분석 (Read, Grep, c4_find_symbol 등)
   - 로그/상태 확인
   - 실험 실행 (필요 시)
4. 다른 팀원과 토론:
   - SendMessage로 발견 사항 공유
   - 반박 증거 제시
5. 결론 도출 후 coordinator에 보고:
   - 가설 지지/반박 여부
   - 증거 요약
   - 권장 조치
6. TaskUpdate(taskId, status="completed")
7. shutdown_request를 받으면 approve로 응답
"""
```

#### 팀원 스폰 실행

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
            mode="plan",  # 읽기 전용
        )
        members.append(name)

elif investigate_mode:
    # C4 상태에서 조사 대상 파악 후 가설 분배
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
    # Standard 모드: 구현 팀원 스폰
    for i in range(member_count):
        name = f"worker-{i+1}"
        Task(
            subagent_type="general-purpose",
            name=name,
            team_name=team_name,
            description=f"C4 Worker {i+1}/{member_count}",
            prompt=MEMBER_PROMPT.format(
                member_name=name, team_name=team_name,
                role_description=f"팀의 {i+1}번째 구현 담당. TaskList에서 미할당 태스크를 선택하여 구현합니다."
            ),
            mode="bypassPermissions",
        )
        members.append(name)
```

모든 팀원은 **동시에 (병렬로) 스폰**하세요. 하나의 메시지에서 여러 Task 호출을 병렬로 보냅니다.

### 5. Coordinator 역할 (당신 = 팀장)

팀 생성 후, coordinator로서 다음을 수행:

```
C4 Swarm 시작! (팀: {team_name})

팀원: {members}
모드: {mode}

팀원 메시지가 자동으로 수신됩니다.
필요 시 SendMessage로 팀원에게 지시할 수 있습니다.

Shift+Tab으로 delegate 모드 전환 가능.
```

**이후 자동 수신되는 팀원 메시지에 반응**:
- 태스크 완료 보고 → 확인 + 다음 태스크 안내
- 차단 보고 → 해결 방안 제시 또는 다른 팀원에 위임
- 질문 → 답변
- 리뷰 결과 → 종합

### 6. 팀 해산

모든 태스크 완료 시:

```python
# 1. 각 팀원에 shutdown 요청
for member in members:
    SendMessage(type="shutdown_request", recipient=member, content="모든 태스크 완료. 수고하셨습니다!")

# 2. 팀원들이 shutdown approve 후
TeamDelete()

print(f"""
C4 Swarm 완료! (팀: {team_name})
  처리된 태스크: N개
  팀원: {len(members)}명
  모드: {mode}
""")
```

---

## 예상 흐름

### Standard 모드
```
/c4-swarm 3
→ C4 상태 확인: EXECUTE, 5개 태스크 실행 가능
→ TeamCreate("c4-1707500000")
→ C4 태스크 5개 → Agent Teams TaskCreate
→ worker-1, worker-2, worker-3 스폰
→ 각 팀원이 TaskList → claim → 구현 → submit → 다음 태스크
→ 팀원 간 필요 시 SendMessage로 협의
→ 모든 태스크 완료 → shutdown → TeamDelete
```

### Review 모드
```
/c4-swarm --review
→ C4 상태에서 최근 변경 파일 확인
→ TeamCreate + 3명 리뷰어 스폰 (Security, Performance, Test)
→ 각 리뷰어가 읽기 전용으로 코드 분석
→ 리뷰 결과를 coordinator에 SendMessage
→ coordinator가 종합 → c4_request_changes 또는 approve
→ shutdown → TeamDelete
```

### Investigate 모드
```
/c4-swarm --investigate
→ 버그/이슈에 대해 3개 가설 수립
→ TeamCreate + 3명 조사관 스폰
→ 각자 독립적으로 가설 검증
→ SendMessage로 증거 공유 + 토론
→ 수렴 후 coordinator에 결론 보고
→ shutdown → TeamDelete
```

---

## 제약사항

| 제약 | 설명 |
|------|------|
| 최대 팀원 | 5명 (안정성 우선) |
| Review 모드 | plan 모드 (읽기 전용) |
| Accept Edits | Standard 모드에서 필수 (`Shift+Tab`) |
| delegate 모드 | coordinator 권장 (`Shift+Tab`) |

## 관련 명령어

- `/c4-run` — 독립 Worker 병렬 실행 (통신 없음, fire-and-forget)
- `/c4-status` — C4 프로젝트 상태 확인
- `/c4-checkpoint` — Checkpoint 리뷰
