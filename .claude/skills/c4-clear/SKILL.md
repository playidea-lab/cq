---
description: |
  Completely reset C4 state for development and debugging. Clears all state
  files, tasks, events, bundles, and locks in .c4/ directory with optional
  config preservation. Use when you need to reset project state, debug
  initialization issues, or start fresh. Triggers: "초기화", "상태 리셋",
  "전체 삭제", "clear c4 state", "reset c4", "delete all tasks",
  "start c4 fresh".
---

# C4 Clear (Development)

C4 상태를 완전히 초기화합니다. 개발/디버깅용입니다.

## Instructions

### Step 1: 현재 상태 확인

1. `mcp__c4__c4_status` 호출
2. 현재 상태 요약 출력:
   ```
   ⚠️  C4 초기화 예정:
   - 프로젝트: {project_id}
   - 상태: {status}
   - 태스크: {pending}개 대기, {done}개 완료
   ```

### Step 2: 확인 요청

```
🗑️  정말 C4 상태를 모두 삭제하시겠습니까?

삭제되는 항목:
- .c4/state.json (상태 파일)
- .c4/tasks.json (태스크 목록)
- .c4/events/ (이벤트 로그)
- .c4/bundles/ (체크포인트 번들)
- .c4/locks/ (잠금 파일)

옵션:
- [Y] 전체 삭제
- [C] config.yaml만 유지
- [N] 취소
```

### Step 3: 실행

사용자 확인 후:

**전체 삭제 (Y):**
```javascript
mcp__c4__c4_clear({ confirm: true })
```

**Config 유지 (C):**
```javascript
mcp__c4__c4_clear({ confirm: true, keep_config: true })
```

### Step 4: 결과 출력

```
✅ C4 상태 초기화 완료

삭제됨:
- .c4/ 디렉토리
- MCP 데몬 캐시

다음 단계:
  /c4-init    - 다시 초기화
```

## Usage

```
/c4-clear           # 확인 후 전체 삭제
/c4-clear --keep-config  # config.yaml 유지
```

## Warning

이 명령은 되돌릴 수 없습니다. 프로덕션 환경에서는 사용하지 마세요.
