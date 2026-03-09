# C9 Run

현재 round의 실험 yaml들을 C5 Hub에 제출하고 완료를 폴링.
워커(pi 서버)가 실험을 실행하고 [C9-START]/[C9-DONE] 마커로 보고.

**트리거**: "c9-run", "실험 제출", "워커 돌려", "run"

## 실행 순서

### Step 1: 현재 라운드 확인
```bash
cat .c9/state.yaml  # round, phase 확인
ls .c9/experiments/ # 제출할 실험 파일 목록
```

### Step 2: 실험 제출 + 폴링
```bash
./scripts/c9-run.sh [round]
```

내부 동작:
1. `.c9/experiments/rN_*.yaml` 파일마다 C5 Hub Job 제출
2. `.c9/rounds/rN/jobs.json`에 job ID 저장
3. 15초마다 job status 폴링
4. 모두 DONE/FAILED → `.c9/rounds/rN/results.txt` 저장
5. state.yaml phase → CHECK

### Step 3: 제출 현황 보고
```
## C9 Run — Round N

제출된 실험:
| 실험명 | Job ID | 상태 |
|--------|--------|------|
| exp_simvq | j-xxx | RUNNING |

예상 완료: ~30분 (훈련) / ~5분 (probe)
```

## 워커 보고 프로토콜

Pi 서버 워커는 표준 마커 출력:
```
[C9-START] exp_name round=N ts=ISO8601
  (실험 진행...)
[C9-DONE] exp_name mpjpe=98.3 pa=69.1 util=0.94
```

## 수동 폴링
```bash
./scripts/c9-run.sh --poll-only
```

## API 정보
- Hub URL: https://piqsol-c5.fly.dev
- API Key: $C5_API_KEY (X-API-Key 헤더)
- Job 상태: GET /v1/jobs/{job_id}
- 로그: GET /v1/jobs/{job_id}/logs
