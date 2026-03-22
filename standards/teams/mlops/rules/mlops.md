# MLOps Team Rules

> ML 모델 운영, 파이프라인, 서빙 규칙.
> 적용: `cq init --team mlops`

---

## 실험 관리

- 모든 실험: 실험 ID 부여 + 결과 기록 (MLflow/W&B 또는 `c4_experiment_record`).
- 재현성: 시드 고정 + 데이터 버전 + 코드 커밋 해시 기록.
- 비교: 베이스라인 대비 개선폭 명시. "좋아졌다"만으로는 불충분.

## 데이터 파이프라인

- 데이터 버전: content hash로 관리 (`hashlib.sha256`).
- 스키마 검증: 파이프라인 입출력에 Pandera 또는 동등 도구.
- 증분 처리: full reload 지양. incremental loading 우선.
- 실패 처리: 파이프라인 단계별 idempotent. 재시도 안전.

## 모델 서빙

- 서빙 포맷: ONNX 또는 TorchScript. 학습 코드 직접 서빙 금지.
- 버전 관리: 모델 레지스트리에 등록 (staging → production).
- A/B 테스트: 신규 모델은 canary로 먼저 검증.
- 롤백: 이전 모델 버전으로 1분 이내 롤백 가능해야.

## 모니터링

- 데이터 드리프트: PSI/KL divergence 기반 모니터링.
- 모델 성능: 예측 정확도 실시간 추적. 임계치 하락 시 알림.
- 리소스: GPU 사용률, 추론 지연시간, 처리량.
- 로깅: 입력/출력 샘플 로깅 (PII 제거 후).

## GPU/학습

- 리소스 요청: GPU 사양/시간 사전 추정 → 팀 리뷰.
- 체크포인팅: 주기적 저장. 장시간 학습 중 중단 대비.
- 분산 학습: DDP 우선. 데이터 병렬 먼저, 모델 병렬은 필요 시.
- 혼합 정밀도: AMP 기본 사용 (메모리/속도 이점).

## Python 필수

- `uv run` 필수. `pip install`, `python script.py` 금지.
- 의존성: `pyproject.toml` + `uv.lock`. requirements.txt 사용 금지.

## CQ 연동 (CQ 프로젝트인 경우)

| 작업 | CQ 도구/스킬 |
|------|-------------|
| 파이프라인/서빙 설계 | `/c4-plan` |
| 학습/파이프라인 실행 | `/c4-run` |
| 실험 결과 기록 | `c4_experiment_record` |
| 자율 실험 루프 | `/c9-loop` |
| 모델/데이터 패턴 조회 | `c4_knowledge_search` |
