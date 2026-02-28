# C9 Report

원격 서버의 실험 결과를 C5 Hub 워커를 통해 수집하고 구조화된 보고서를 생성.
선택적으로 c9-conference를 자동 트리거해서 결과 해석 토론을 이어갈 수 있음.

**트리거**: "c9-report", "실험 보고", "결과 보고해", "report 해줘"

## 실행 순서

### Step 1: 실험 경로 확인
- 기본 경로: `/home/pi/git/hmr_unified/experiments/paper1/`
- 추가 경로: `/home/pi/git/hmr_postprocRL/EXPERIMENT_TRACKER.md`
- 사용자가 다른 경로 지정 시 해당 경로 사용

### Step 2: C5 Hub Job 제출
```bash
curl -X POST https://piqsol-c5.fly.dev/v1/jobs/submit \
  -H "X-API-Key: cq-test-key-2026" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "c9-report",
    "command": "python3 -c \"[메트릭 수집 스크립트]\"",
    "tags": ["c9", "report"]
  }'
```

Python 스크립트 패턴:
```python
import json, os, glob
base = '/home/pi/git/hmr_unified/experiments/paper1'
dirs = sorted(glob.glob(base + '/exp056*'))
results = []
for d in dirs:
    f = d + '/eval_exp056_results.json'
    if os.path.exists(f):
        r = json.load(open(f))
        results.append((os.path.basename(d), r['mpjpe_mean_mm'], r['pa_mpjpe_mean_mm']))
results.sort(key=lambda x: x[1])
for name, mpjpe, pa in results:
    print(f'{name}|{mpjpe:.1f}|{pa:.1f}')
```

### Step 3: 결과 파싱 및 보고서 생성

보고서 형식:
```
## C9 Report — [날짜]

### 실험 현황
- 완료된 실험: N개
- 베이스라인: MPJPE Xmm / PA-MPJPE Xmm

### 성능 순위 (MPJPE 기준)
| 순위 | 실험명 | MPJPE | PA-MPJPE | vs 베이스라인 |
|------|--------|-------|----------|--------------|
| 1 | ... | ... | ... | ... |

### 주요 발견
- Best config: ...
- 실패한 방향: ...
- 미탐색 영역: ...

### 다음 c9-conference 주제 제안
- "[실험 결과에서 도출된 핵심 질문]"
```

### Step 4: c9-conference 트리거 (선택)
사용자가 "토론해줘" 또는 "왜 이런 결과?" 요청 시 →
수집된 데이터를 컨텍스트로 `/c9-conference` 자동 실행.

## API 정보
- Hub URL: `https://piqsol-c5.fly.dev`
- API Key: `cq-test-key-2026` (X-API-Key 헤더)
- 로그 조회: `GET /v1/jobs/{job_id}/logs`
- Job 결과 대기: 10-15초 후 조회
