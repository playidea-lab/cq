# C4 Swarm (Deprecated)

> ⚠️ **DEPRECATED**: `/c4-swarm`은 `/c4-run`으로 통합되었습니다.

## 마이그레이션

```bash
# 이전
/c4-swarm 4      # 4개 Worker 스폰

# 이후 (동일한 기능)
/c4-run 4        # 4개 Worker 스폰
/c4-run          # 자동 분석 후 최적 Worker 수 스폰 (권장)
/c4-run --max 4  # 최대 4개로 제한하여 자동 스폰
```

## /c4-run의 새 기능

- **자동 병렬도 분석**: 의존성 그래프를 분석하여 최적 Worker 수 추천
- **기본값 = 자동**: 인자 없이 실행하면 자동으로 최적 수 스폰
- **통합된 UX**: 단일/멀티 Worker 모두 하나의 명령어로

## Instructions

사용자가 `/c4-swarm`을 호출하면:

```python
print("""
⚠️ /c4-swarm은 /c4-run으로 통합되었습니다.

사용법:
  /c4-run        # 자동 분석 후 최적 Worker 수 스폰 (권장)
  /c4-run 4      # 4개 Worker 스폰
  /c4-run --max 4  # 최대 4개로 제한

지금 /c4-run을 실행할까요? [Y/n]
""")

# 사용자 확인 후 /c4-run 실행
```

실제로는 **사용자에게 `/c4-run` 사용을 안내**하고, 원하면 바로 실행해줍니다.
