# C4 Add Task

Add a new task to the project queue.

## Arguments

```
/c4-add-task <task-id> "<title>" "<dod>" [scope]
```

- `task-id`: Unique task identifier (e.g., T-001, FEAT-01)
- `title`: Brief task title
- `dod`: Definition of Done - clear completion criteria
- `scope`: (Optional) File/directory scope for the task

## Instructions

1. Parse the arguments from `$ARGUMENTS`
2. Call `mcp__c4__c4_add_todo` with:
   - task_id
   - title
   - dod
   - scope (if provided)
3. Call `mcp__c4__c4_status` to show updated queue
4. Remind user to update `docs/CHECKPOINTS.md` if this task is part of a checkpoint

## Usage Examples

```
/c4-add-task T-001 "Implement login" "Login form works with validation"
/c4-add-task T-002 "Add tests" "80% coverage" src/auth/
```

## Best Practices

- Use clear, unique task IDs
- **DoD는 반드시 구체적이고 검증 가능하게 작성** (아래 예시 참조)
- Define scope to prevent conflicts in multi-worker setups

## ⚠️ DoD 작성 원칙 (필수!)

**좋은 DoD의 조건:**
1. **검증 가능**: "~가 동작한다", "~를 반환한다"
2. **구체적**: 모호한 표현 금지 ("개선", "최적화" ❌)
3. **독립적**: 이 태스크만으로 확인 가능

| ❌ 나쁜 DoD | ✅ 좋은 DoD |
|------------|------------|
| "로그인 구현" | "이메일/비밀번호 입력 시 JWT 반환, 실패 시 401" |
| "테스트 추가" | "auth 모듈 테스트 5개 추가, 커버리지 80% 이상" |
| "버그 수정" | "null 입력 시 빈 배열 반환, 테스트 추가" |
