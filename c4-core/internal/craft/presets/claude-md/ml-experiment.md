---
name: ml-experiment
description: ML 실험 프로젝트용 CLAUDE.md — uv, PyTorch, 실험 추적, 재현성 컨벤션
---

# Project Instructions

## Environment & Run

```bash
# 의존성 설치
uv sync

# 스크립트 실행
uv run python train.py
uv run pytest tests/

# 코드 품질
uv run ruff check .
uv run mypy src/
```

**NEVER use**: `python script.py`, `pip install`

## Experiment Tracking

- 모든 실험에 고유 ID 부여: `exp{NNN}` (예: exp001)
- 하이퍼파라미터는 config 파일 (YAML/JSON)로 관리
- 메트릭 출력은 `@key=value` 형식으로 stdout에 포함

```python
# MetricWriter 호환 출력
print(f"Epoch {epoch} @train_loss={loss:.4f} @val_acc={acc:.4f}")
```

## Reproducibility

```python
import random, numpy as np, torch

def set_seed(seed: int = 42):
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(seed)
```

- 데이터 버전: `hashlib.sha256(data.tobytes()).hexdigest()[:8]`
- 모델 체크포인트: `checkpoints/{exp_id}_best.pt`

## Code Style

- Type hints 필수 (`from __future__ import annotations`)
- Pathlib 사용 (`Path` not `os.path`)
- 데이터 검증: Pydantic 모델 사용
- 대용량 데이터: generator/DataLoader, 메모리 전부 로드 금지

## Project Structure

```
src/
  data/        — 데이터 로더, 전처리
  models/      — 모델 정의
  training/    — 학습 루프
  evaluation/  — 평가 메트릭
configs/       — 실험 설정 YAML
experiments/   — 실험 결과 저장
tests/         — 단위 테스트
```

# CUSTOMIZE: 실험 추적 도구 (MLflow/W&B/CQ), 모델 저장 경로, GPU 설정, 데이터셋 경로
