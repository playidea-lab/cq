# MCP Server 코드 리뷰

> **파일**: `c4/mcp_server.py`
> **라인 수**: 5,540줄
> **리뷰 일자**: 2026-02-01
> **Task ID**: T-001-0

## 요약

MCP Server는 C4의 핵심 컴포넌트로, 모든 MCP 도구를 제공합니다. 기능적으로는 잘 작동하지만, 파일 크기와 복잡도가 높아 리팩토링이 필요합니다.

## 주요 발견 사항

### 1. 파일 크기 문제 (Critical)

- **5,540줄**은 단일 파일로서 매우 큼
- 읽기, 유지보수, 테스트가 어려움
- 권장: 1,000줄 이하로 분리

**제안**: 다음과 같이 분리
```
c4/mcp/
├── __init__.py
├── server.py         # Server 초기화, call_tool 라우팅
├── daemon.py         # C4Daemon 클래스
├── tools/
│   ├── status.py     # c4_status, c4_clear 등
│   ├── task.py       # c4_get_task, c4_submit 등
│   ├── discovery.py  # c4_save_spec, c4_discovery_complete 등
│   ├── design.py     # c4_save_design, c4_design_complete 등
│   ├── checkpoint.py # c4_checkpoint 등
│   └── agent.py      # c4_query_agent_graph 등
└── utils.py          # 공통 유틸리티
```

### 2. 복잡도 초과 함수 (High)

| 함수 | 복잡도 | 권장 |
|------|--------|------|
| `create_server` | 40 | < 10 |
| `c4_get_task` | 38 | < 10 |
| `call_tool` | 30 | < 10 |
| `c4_submit` | 23 | < 10 |
| `c4_checkpoint` | 14 | < 10 |

**제안**:
- 각 함수를 작은 헬퍼 함수로 분해
- 전략 패턴 또는 핸들러 맵 사용

### 3. Lazy Import 과다 (Medium)

47개의 함수 내 import 문 발견:
```python
def some_method(self):
    import os  # ← lazy import
    import yaml
```

**장점**: 시작 시간 최적화
**단점**: 코드 중복, 표준 위반

**제안**:
- 자주 사용되는 모듈은 상단으로 이동
- 드물게 사용되는 것만 lazy import 유지

### 4. 예외 처리 (Medium)

27개의 blind except 발견:
```python
try:
    ...
except Exception:  # ← too broad
    ...
```

**제안**: 구체적인 예외 타입 사용

### 5. 로깅 f-string (Low)

45개의 f-string 로깅:
```python
logger.info(f"Message {var}")  # ← 성능 이슈
```

**제안**: lazy formatting 사용
```python
logger.info("Message %s", var)
```

### 6. Private Member 접근 (Low)

14개의 `_` 접두사 멤버 외부 접근:
```python
daemon._tasks  # ← SLF001
```

**제안**: 필요시 public getter/setter 추가

## 보안 검토

| 항목 | 상태 | 비고 |
|------|------|------|
| SQL Injection | ✅ 안전 | 파라미터화된 쿼리 사용 |
| Path Traversal | ✅ 안전 | Path 객체 사용 |
| Command Injection | ⚠️ 주의 | subprocess 사용 시 shell=False 확인 필요 |
| Secrets Exposure | ✅ 안전 | 민감 정보 로깅 없음 |

## 테스트 커버리지

```bash
uv run pytest tests/unit/test_mcp_server.py -v --cov=c4/mcp_server
```

현재 테스트 파일이 존재하지 않거나 별도 위치에 있을 수 있음.

## 권장 조치

### 즉시 (P1)
1. [ ] 복잡도 초과 함수 분해 (`c4_get_task`, `c4_submit`)
2. [ ] blind except를 구체적 예외로 변경

### 단기 (P2)
1. [ ] 파일을 모듈로 분리
2. [ ] lazy import 정리

### 장기 (P3)
1. [ ] 로깅 f-string → lazy formatting
2. [ ] private member 접근 정리
3. [ ] 단위 테스트 추가

## 메트릭

| 메트릭 | 값 | 상태 |
|--------|-----|------|
| 라인 수 | 5,540 | ❌ 초과 (권장 < 1,000) |
| 함수 수 | ~80 | ⚠️ 많음 |
| 최대 복잡도 | 40 | ❌ 초과 (권장 < 10) |
| 린트 이슈 | ~150 | ⚠️ 개선 필요 |

## 결론

MCP Server는 기능적으로 잘 작동하지만, **리팩토링이 필요**합니다. 특히 파일 분리와 복잡도 감소가 우선입니다.
