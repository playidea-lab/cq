---
name: perf-check
description: |
  성능 점검 체크리스트. N+1 쿼리, 캐시 미스, 메모리 누수, 느린 API 탐지.
  트리거: "성능 점검", "perf check", "느린 쿼리", "N+1", "성능 이슈"
allowed-tools: Read, Glob, Grep, Bash
---
# Performance Check

서비스 성능 병목을 체계적으로 탐지합니다.

## 실행 순서

### Step 1: 현재 성능 측정

```bash
# API 응답 시간 확인
curl -o /dev/null -s -w "총: %{time_total}s, 연결: %{time_connect}s\n" \
  http://localhost:<port>/<endpoint>

# 부하 테스트 (기본)
ab -n 100 -c 10 http://localhost:<port>/<endpoint>
# 또는
k6 run --vus 10 --duration 30s <script.js>
```

### Step 2: DB 쿼리 분석

#### N+1 쿼리 탐지

```bash
# ORM 사용 시 로그에서 N+1 패턴 확인
# 루프 안에서 DB 호출하는 패턴 탐색
grep -rn "for.*range\|forEach\|for.*in" --include="*.go" --include="*.py" . | \
  xargs grep -l "Find\|Query\|Get\|fetch"
```

- [ ] 루프 내 쿼리 없음 (N+1 없음)
- [ ] JOIN으로 대체 가능한 다중 쿼리 없음
- [ ] 대량 조회 시 pagination 적용

#### 느린 쿼리 확인

```sql
-- PostgreSQL: 느린 쿼리 로그
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;

-- 실행 계획
EXPLAIN ANALYZE <query>;
```

- [ ] 인덱스 없는 WHERE 조건 없음
- [ ] Full Table Scan 없음
- [ ] 쿼리 실행 시간 < 100ms

### Step 3: 캐시 활용 점검

```bash
# 캐시 히트율 확인 (Redis)
redis-cli info stats | grep hit

# 캐시 없이 반복 조회되는 패턴 탐색
grep -rn "cache\|Cache\|redis\|Redis" --include="*.go" .
```

- [ ] 자주 읽는 데이터에 캐시 적용
- [ ] 캐시 만료 정책 설정
- [ ] 캐시 무효화 로직 존재

### Step 4: 메모리 분석

```bash
# Go pprof
go tool pprof http://localhost:<port>/debug/pprof/heap

# Python memory
uv run python -m memory_profiler <script>
```

- [ ] 메모리 누수 없음
- [ ] 대용량 객체 명시적 해제
- [ ] 불필요한 전역 변수 없음

### Step 5: 동시성 이슈

```bash
# Go race condition
go test -race ./...

# 고루틴 누수 확인
curl http://localhost:<port>/debug/pprof/goroutine?debug=2
```

- [ ] Race condition 없음
- [ ] 고루틴 누수 없음
- [ ] 데드락 없음

### Step 6: 결과 정리

```
## 성능 점검 결과

**측정 일시**: YYYY-MM-DD
**환경**: staging / production

### 발견된 문제
1. <문제> — <영향> — <해결 방안>

### 성능 지표
- P50 응답: Xms
- P95 응답: Xms
- DB 쿼리 평균: Xms
- 메모리: XMB

### 액션 아이템
- [ ] 우선순위1: ...
- [ ] 우선순위2: ...
```

# CUSTOMIZE: 성능 기준값 설정 (SLA), 모니터링 도구 (Datadog, Grafana), 프로파일링 환경
