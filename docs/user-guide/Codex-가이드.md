# Codex CLI 가이드

C4를 Codex CLI에서 실무적으로 운용하는 방법입니다. 핵심은 `Direct`와 `Worker`를 명시적으로 분리하는 것입니다.

## 1. 빠른 설정

프로젝트 루트에서 실행:

```bash
# 1) c4 MCP 바이너리 빌드
cd c4-core && go build -o bin/c4 ./cmd/c4 && cd ..

# 2) Codex MCP 설정 반영
scripts/codex/setup-mcp.sh --write

# 3) 프로젝트 Codex 에이전트 설치
scripts/codex/install-agents.sh
```

확인:

```bash
codex
# 대화에서 c4_status() 호출
```

## 2. 시나리오 A: Direct 모드 (권장 시작점)

사용 조건:
- 파일 간 결합이 크고, 한 세션에서 끝까지 잡고 가야 하는 작업
- 리팩토링/마이그레이션/횡단 수정

실행 순서:
1. `c4_status()`로 `ready_task_ids` 확인
2. `c4_claim(task_id="...")`
3. 구현
4. `c4_run_validation(names=["lint","unit"])`
5. 커밋
6. `c4_report(task_id="...", summary="...", files_changed=[...])`

주의:
- Direct 태스크는 `c4_submit`이 서버에서 거절됩니다.
- `c4_claim` 응답의 `suggested_validations`, `next_steps`를 그대로 따르는 것이 가장 빠릅니다.

## 3. 시나리오 B: Worker 모드 (병렬 처리)

사용 조건:
- 독립 scope 태스크가 여러 개 있고 병렬 처리 이득이 큰 경우

실행 순서:
1. `c4_status()` 확인
2. 필요 시 `c4_start()`
3. 반복:
   - `c4_get_task(worker_id="codex-worker-...")`
   - 구현
   - `c4_run_validation(...)`
   - 커밋
   - `c4_submit(task_id, worker_id, commit_sha, validation_results)`

주의:
- `c4_submit`은 `in_progress` + owner 검증이 강제됩니다.
- Direct owner 태스크를 submit하면 거절됩니다 (`c4_report` 사용).

## 4. 시나리오 C: Hybrid 운용

추천 패턴:
- 낮 시간: Worker 모드로 독립 태스크 소모
- 핵심 변경 구간: Direct 모드로 전환
- 게이트: `c4_checkpoint`로 의사결정 고정

## 5. Claude Code 대비 Codex 운영 포인트

| 항목 | Claude Code | Codex CLI |
|------|-------------|-----------|
| 커맨드 진입 | 슬래시 명령어 | 트리거 기반 agent + MCP 직접 호출 |
| 강점 | UI/훅 기반 자동화 | 터미널 스크립트 결합, 명시적 제어 |
| Direct 안전성 | 프롬프트 규율 중심 | 서버 가드 + agent 규칙 동시 적용 |
| 권장 전략 | `/c4-run` 중심 | Direct 우선 + Worker 선택적 사용 |

## 6. 동등/우위 동작 구성 가능성

결론:
- **동등 구성 가능**: 주요 C4 MCP 도구(62개) 동일 사용
- **우위 가능 영역**: Codex는 셸/스크립트 결합이 쉬워 Direct 모드의 반복 작업 표준화가 쉽습니다

필수 조건:
- `.codex/agents`를 프로젝트 기준으로 관리
- `scripts/codex/setup-mcp.sh`로 설정 편차 제거
- Direct/Worker 혼용 시 owner 규칙(`claim/report` vs `get_task/submit`) 엄수
