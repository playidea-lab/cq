# Worktree 가이드

> **멀티 Worker 환경에서 Git 충돌 없이 병렬 작업하기**

---

## 개요

C4 Worktree 모드는 각 Worker가 독립된 Git worktree에서 작업하도록 하여 파일 충돌 없이 병렬 작업을 가능하게 합니다.

### 왜 Worktree가 필요한가?

| 문제 | 해결 |
|------|------|
| **Git 충돌** | 여러 Worker가 같은 파일 수정 시 충돌 발생 | 각 Worker가 독립된 작업 공간 사용 |
| **브랜치 경쟁** | 동시에 같은 브랜치에서 작업 불가 | 각 Worker별 브랜치 자동 생성 |
| **상태 오염** | 한 Worker의 변경이 다른 Worker에 영향 | 완전 격리된 디렉토리 구조 |

### 핵심 개념

```
Main Repository (프로젝트 루트)
├── .c4/
│   └── worktrees/
│       ├── worker-abc123/     # Worker 1의 격리된 작업 공간
│       │   ├── .git           # Git 링크 (공유)
│       │   ├── src/           # 독립된 파일 복사본
│       │   └── ...
│       └── worker-def456/     # Worker 2의 격리된 작업 공간
│           ├── .git
│           ├── src/
│           └── ...
└── src/                       # 메인 프로젝트 (Worker가 직접 건드리지 않음)
```

---

## 설정

### 기본 설정

Worktree 모드는 기본으로 활성화되어 있습니다. `.c4/config.yaml`에서 설정을 조정할 수 있습니다:

```yaml
# .c4/config.yaml
worktree:
  enabled: true              # Worktree 격리 사용 (기본: true)
  base_branch: main          # 기준 브랜치
  work_dir: null             # null이면 기본 경로 (.c4/worktrees/)
  auto_cleanup: true         # 태스크 완료 후 자동 정리
  completion_action: merge   # 완료 시 동작: 'pr' 또는 'merge'
```

### 설정 옵션 상세

| 옵션 | 기본값 | 설명 |
|------|--------|------|
| `enabled` | `true` | Worktree 모드 활성화/비활성화 |
| `base_branch` | `main` | 새 worktree 생성 시 기준 브랜치 |
| `work_dir` | `.c4/worktrees/` | worktree가 저장될 디렉토리 |
| `auto_cleanup` | `true` | 태스크 완료 후 worktree 자동 삭제 |
| `completion_action` | `merge` | `merge`: 직접 병합, `pr`: PR 생성 |

### 비활성화 (권장하지 않음)

```yaml
worktree:
  enabled: false  # 단일 Worker 또는 순차 실행에만 사용
```

---

## 동작 방식

### 1. 태스크 할당 시

Worker가 `c4_get_task()`를 호출하면:

```python
task = c4_get_task(worker_id="worker-abc123")

# 응답에 worktree 경로 포함
print(task.worktree_path)  # /path/to/project/.c4/worktrees/worker-abc123
print(task.branch)         # c4/w-T-001-0
```

### 2. Worktree 생성 자동화

C4가 자동으로 처리:

1. Worker별 디렉토리 생성 (`.c4/worktrees/worker-abc123/`)
2. Git worktree 연결
3. 태스크 브랜치 생성 (`c4/w-T-001-0`)
4. 기준 브랜치에서 코드 복사

### 3. 작업 수행

Worker는 `worktree_path`에서만 작업합니다:

```python
# Worker 루프 예시
task = c4_get_task(worker_id)

if task.worktree_path:
    # worktree에서 작업
    work_dir = task.worktree_path
else:
    # 메인 디렉토리에서 작업 (단일 Worker)
    work_dir = "."

# 이 경로에서만 파일 수정
implement_task(work_dir, task)
```

### 4. 커밋 및 제출

```python
# worktree 내에서 커밋
git_commit(work_dir, message="[T-001-0] 구현 완료")

# C4에 제출
c4_submit(task_id, commit_sha=commit_sha)
```

### 5. 병합 및 정리

모든 태스크 완료 시:
- `completion_action: merge` → 자동으로 `base_branch`에 병합
- `completion_action: pr` → PR 자동 생성
- `auto_cleanup: true` → worktree 디렉토리 삭제

---

## 브랜치 전략

### 브랜치 네이밍

```
{work_branch_prefix}{task_id}
```

예시:
- `c4/w-T-001-0` - 첫 번째 태스크
- `c4/w-R-001-0` - 리뷰 태스크
- `c4/w-T-001-1` - 수정 버전

### 브랜치 흐름

```
main
  │
  ├── c4/w-T-001-0 (Worker 1)
  │     └── 완료 → main으로 병합
  │
  ├── c4/w-T-002-0 (Worker 2)
  │     └── 완료 → main으로 병합
  │
  └── c4/w-T-003-0 (Worker 3)
        └── 완료 → main으로 병합
```

### 별도 작업 브랜치 사용 (선택)

PR 워크플로우를 원하면:

```yaml
# .c4/config.yaml
worktree:
  base_branch: work          # 'main' 대신 'work' 브랜치 사용
  completion_action: pr      # 완료 시 work → main PR 생성
```

이 경우 브랜치 구조:

```
main (기본 브랜치)
  │
  └── work (base_branch)
        │
        ├── c4/w-T-001-0
        ├── c4/w-T-002-0
        └── c4/w-T-003-0
              │
              └── 모든 태스크 완료 → work → main PR 생성
```

---

## MCP 도구

### c4_get_task 응답

```python
{
    "task_id": "T-001-0",
    "title": "API 엔드포인트 구현",
    "branch": "c4/w-T-001-0",
    "worktree_path": "/path/to/.c4/worktrees/worker-abc123",
    # ... 기타 태스크 정보
}
```

### c4_status에서 Worktree 확인

```python
status = c4_status()

# Worker별 worktree 상태
for worker_id, info in status["workers"].items():
    if info["state"] == "busy":
        print(f"{worker_id}: {info['task_id']}")
```

### Worktree 정리 (수동)

```bash
# 모든 worktree 정리
rm -rf .c4/worktrees/*

# 또는 Git 명령어 사용
git worktree prune
git worktree list
```

---

## 멀티 Worker 실행

### Smart Auto Mode (`/run`)

```bash
/run           # 자동으로 적절한 수의 Worker 스폰
/run 3         # 3개 Worker 스폰
/run --max 4   # 최대 4개로 제한
```

C4가 자동으로:
1. 태스크 의존성 분석
2. 병렬화 가능한 태스크 식별
3. 각 Worker에게 독립된 worktree 할당
4. 동시 실행

### 단일 Worker 모드

```bash
/run 1         # 이 세션에서 직접 실행
```

단일 Worker 시에도 worktree가 생성되지만, 메인 디렉토리에서 직접 작업할 수 있습니다.

---

## 주의사항

### 1. Scope Lock과 함께 사용

같은 `scope`(파일/디렉토리)의 태스크는 순차 실행됩니다:

```python
# 태스크 정의 시
c4_add_todo(
    title="User 모델 구현",
    scope="src/models/user.py",  # 이 파일을 수정하는 다른 태스크와 동시 실행 안됨
    ...
)
```

### 2. 디스크 공간

각 worktree는 프로젝트의 복사본이므로 디스크 공간을 사용합니다:
- Git 객체는 공유 (`.git/objects/`)
- 작업 파일은 복사됨

대규모 프로젝트에서는 `auto_cleanup: true` 권장.

### 3. IDE 설정

worktree 디렉토리를 IDE에서 무시하도록 설정:

```gitignore
# .gitignore (자동 추가됨)
.c4/worktrees/
```

---

## 문제 해결

### Worktree가 이미 존재함

```
fatal: '.c4/worktrees/worker-abc123' already exists
```

**해결**:
```bash
# 수동으로 정리
rm -rf .c4/worktrees/worker-abc123

# 또는 Git worktree 정리
git worktree prune
```

### 병합 충돌

여러 Worker가 완료 후 병합 시 충돌 발생 가능:

**해결**:
1. 충돌 브랜치 확인: `git branch -a | grep c4/w-`
2. 수동으로 충돌 해결
3. 병합 완료

### Worker가 잘못된 경로에서 작업

Worker가 `worktree_path`를 무시하고 메인 디렉토리에서 작업하면 충돌 발생.

**확인**: Worker 프롬프트에서 `worktree_path` 사용 여부 검토.

### 브랜치가 삭제되지 않음

`auto_cleanup`이 worktree 디렉토리만 삭제하고 브랜치는 유지합니다.

**정리**:
```bash
# 병합된 브랜치 삭제
git branch -d c4/w-T-001-0

# 모든 C4 브랜치 삭제 (주의!)
git branch | grep 'c4/w-' | xargs git branch -D
```

---

## Best Practices

### 1. 항상 Worktree 사용

멀티 Worker 실행 시 worktree를 비활성화하면 충돌이 발생합니다.

### 2. Scope 명확히 지정

태스크 정의 시 `scope`를 명확히 지정하여 Lock 충돌을 방지:

```python
c4_add_todo(
    title="API 구현",
    scope="src/api/",  # 디렉토리 단위
    ...
)
```

### 3. 자동 정리 활성화

디스크 공간 절약을 위해 `auto_cleanup: true` 유지.

### 4. PR 워크플로우 고려

팀 리뷰가 필요하면 `completion_action: pr` 사용:

```yaml
worktree:
  base_branch: develop
  completion_action: pr
```

---

## 관련 문서

- [Smart Auto Mode](Smart-Auto-Mode.md) - 병렬 실행 상세
- [워크플로우 개요](워크플로우-개요.md) - 전체 C4 워크플로우
- [문제 해결](문제-해결.md) - FAQ 및 트러블슈팅
