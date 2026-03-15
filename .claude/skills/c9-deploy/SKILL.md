---
name: c9-deploy
description: |
  C9 연구 완료 후 모델을 배포합니다.
  트리거: "c9-deploy", "모델 배포", "deploy model"
---
# C9 Deploy

Best model을 edge 서버에 배포. c9-finish와 독립적으로 실행 가능.

**트리거**: "c9-deploy", "edge 배포", "모델 배포", "deploy"

## 전제 조건
- best model 파일이 pi 서버에 존재 (c9-finish 또는 수동 지정)
- C5 Hub edge worker가 대상 서버에 등록되어 있음

## 실행 순서

### Step 1: 배포 대상 확인
```bash
cat .c9/state.yaml  # finish.best_model_path 확인
# 또는 사용자가 직접 지정: "exp_simvq 모델 배포해줘"
```

### Step 2: Edge 배포 Job 제출
```bash
# C5 Hub edge deploy
curl -X POST https://piqsol-c5.fly.dev/v1/jobs/submit \
  -H "X-API-Key: $C5_API_KEY" \
  -d '{
    "name": "c9-deploy",
    "command": "...",
    "tags": ["c9", "deploy", "edge"]
  }'
```

배포 내용:
1. best_checkpoint.pt → edge 서버 `/models/hmr_unified_latest.pt`
2. config.json → `/models/hmr_unified_config.json`
3. 서비스 재시작 (있을 경우)

### Step 3: 배포 검증
```bash
# edge 서버에서 추론 테스트
# [C9-DEPLOY-OK] 마커 확인
```

### Step 4: 배포 기록
`.c9/DEPLOY_LOG.md`에 추가:
```markdown
## Deploy [날짜]
- Model: exp_name (Round N, MPJPE=Xmm)
- Target: edge server
- Status: SUCCESS
```

## 주의
- 배포는 state.yaml phase와 무관하게 언제든 실행 가능
- FINISH 없이도 특정 실험 모델을 직접 배포 가능:
  "Round 1의 exp_simvq 모델 바로 배포해줘" → Step 2로 직행
