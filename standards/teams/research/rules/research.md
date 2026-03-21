# Research Team Rules

> 딥러닝 연구, 논문 실험, 프로토타이핑 규칙.
> 적용: `cq init --team research`

---

## 실험 프로토콜

- 실험 ID: 순차 부여 (`expXXX`). Paper별 범위 분리.
- 실험 시작: tracker에 등록 → 가설 명시 → 실행.
- 실험 완료: 결과 기록 → 베이스라인 비교 → 커밋.
- 실패한 실험도 기록. "이 방법은 안 된다"도 결과.

## 코드 구조

- 학습 스크립트: config 기반. 하드코딩된 하이퍼파라미터 금지.
- config: YAML 또는 dataclass. CLI argument와 병용 가능.
- 모델 정의: 모듈화. forward만 있는 거대 클래스 금지.
- 데이터 로더: 재현 가능하게 (시드 고정, worker 수 명시).

## 재현성

- 시드: `torch.manual_seed`, `np.random.seed`, `random.seed` 모두 설정.
- CUDA: `torch.backends.cudnn.deterministic = True` (디버깅 시).
- 환경: `uv.lock` 커밋. 실험 시점 환경 재현 가능.
- 데이터: 전처리 파이프라인 버전화. raw → processed 경로 추적.

## 논문 작업

- 수치 보고: 평균 ± 표준편차 (최소 3회 반복).
- 비교 테이블: 동일 데이터셋/메트릭/전처리 조건에서.
- Ablation: 핵심 주장마다 ablation study 수행.
- 그래프: 300 DPI, matplotlib 기본 설정 통일.

## 프로토타이핑 vs 프로덕션

- 연구 코드는 빠른 반복이 우선. 과도한 추상화 금지.
- 프로덕션 전환 결정 시: MLOps 팀과 협업 → 서빙 규격으로 변환.
- Jupyter 노트북: 탐색/시각화용. 학습 코드는 `.py`로 분리.

## Python 필수

- `uv run` 필수. `pip install`, `python script.py` 금지.
- PyTorch 버전: 팀 표준 버전 사용. 개인 버전 금지.
