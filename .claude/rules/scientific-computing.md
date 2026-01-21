# Scientific Computing Rules

> ML/DL, 데이터 과학, 연구 프로젝트를 위한 과학적 컴퓨팅 규칙

---

## 핵심 원칙

### 1. 재현성 (Reproducibility)

> "재현할 수 없으면 과학이 아니다."

```python
# ✅ GOOD: 모든 랜덤 시드 고정
import random
import numpy as np
import torch

SEED = 42
random.seed(SEED)
np.random.seed(SEED)
torch.manual_seed(SEED)
torch.cuda.manual_seed_all(SEED)
torch.backends.cudnn.deterministic = True
```

```python
# ✅ GOOD: 실험 스냅샷 저장
experiment = {
    "data_version": hashlib.sha256(data.tobytes()).hexdigest()[:8],
    "code_commit": subprocess.check_output(["git", "rev-parse", "HEAD"]).decode().strip()[:7],
    "config": config.dict(),
    "timestamp": datetime.now().isoformat(),
    "results": results,
}
```

### 2. 버전 관리

| 대상 | 방법 |
|------|------|
| **코드** | Git commit hash |
| **데이터** | Content hash (SHA256) |
| **모델** | 체크포인트 + config |
| **환경** | `uv.lock`, `requirements.txt` |

```bash
# 데이터 버전 생성
sha256sum data/train.csv > data/train.csv.sha256
```

### 3. 실험 추적

```python
# ✅ GOOD: 구조화된 실험 로그
# experiments/exp001_baseline.yaml
"""
experiment:
  id: exp001
  name: "RandomForest Baseline"
  date: 2025-01-21

hypothesis: "RF가 XGBoost보다 빠르면서 비슷한 성능"

data:
  train: "sha256:abc123..."
  test: "sha256:def456..."

config:
  model: RandomForestClassifier
  n_estimators: 100
  max_depth: 10
  random_state: 42

results:
  accuracy: 0.847
  f1_score: 0.823
  train_time: 12.3s

conclusion: "가설 확인됨, baseline으로 채택"
"""
```

---

## 데이터 처리

### 데이터 검증

```python
import pandera as pa
from pandera import Column, Check

# ✅ GOOD: 스키마 정의
schema = pa.DataFrameSchema({
    "age": Column(int, Check.in_range(0, 120)),
    "income": Column(float, Check.ge(0)),
    "category": Column(str, Check.isin(["A", "B", "C"])),
})

# 검증
validated_df = schema.validate(df)
```

### 데이터 분할

```python
from sklearn.model_selection import train_test_split

# ✅ GOOD: 시드 고정 + stratify
X_train, X_test, y_train, y_test = train_test_split(
    X, y,
    test_size=0.2,
    random_state=42,
    stratify=y,  # 클래스 비율 유지
)

# ✅ GOOD: 시계열은 시간순 분할
train = df[df["date"] < "2024-01-01"]
test = df[df["date"] >= "2024-01-01"]
```

### 전처리 파이프라인

```python
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler
from sklearn.compose import ColumnTransformer

# ✅ GOOD: 파이프라인으로 누수 방지
preprocessor = ColumnTransformer([
    ("num", StandardScaler(), numeric_cols),
    ("cat", OneHotEncoder(), categorical_cols),
])

pipeline = Pipeline([
    ("preprocess", preprocessor),
    ("model", RandomForestClassifier()),
])

# fit은 train에만!
pipeline.fit(X_train, y_train)
```

---

## 모델 개발

### 실험 구조

```
experiments/
├── exp001_baseline/
│   ├── config.yaml      # 설정
│   ├── train.py         # 학습 코드
│   ├── metrics.json     # 결과
│   └── model.pkl        # 체크포인트
├── exp002_tuning/
└── exp003_ensemble/
```

### 모델 저장

```python
import joblib
from pathlib import Path

def save_experiment(model, config, metrics, exp_dir):
    exp_dir = Path(exp_dir)
    exp_dir.mkdir(parents=True, exist_ok=True)

    # 모델
    joblib.dump(model, exp_dir / "model.pkl")

    # 설정
    with open(exp_dir / "config.yaml", "w") as f:
        yaml.dump(config, f)

    # 결과
    with open(exp_dir / "metrics.json", "w") as f:
        json.dump(metrics, f, indent=2)

    # 재현성 정보
    with open(exp_dir / "reproducibility.json", "w") as f:
        json.dump({
            "git_commit": get_git_commit(),
            "timestamp": datetime.now().isoformat(),
            "python_version": sys.version,
        }, f, indent=2)
```

### 하이퍼파라미터 튜닝

```python
from sklearn.model_selection import cross_val_score
import optuna

def objective(trial):
    params = {
        "n_estimators": trial.suggest_int("n_estimators", 50, 300),
        "max_depth": trial.suggest_int("max_depth", 3, 15),
        "min_samples_split": trial.suggest_int("min_samples_split", 2, 20),
    }

    model = RandomForestClassifier(**params, random_state=42)
    scores = cross_val_score(model, X_train, y_train, cv=5, scoring="f1")

    return scores.mean()

# ✅ GOOD: 시드 고정
study = optuna.create_study(
    direction="maximize",
    sampler=optuna.samplers.TPESampler(seed=42),
)
study.optimize(objective, n_trials=100)
```

---

## 딥러닝 특화

### PyTorch 설정

```python
import torch
import torch.nn as nn

# ✅ GOOD: 재현성 설정
def set_seed(seed=42):
    torch.manual_seed(seed)
    torch.cuda.manual_seed_all(seed)
    torch.backends.cudnn.deterministic = True
    torch.backends.cudnn.benchmark = False
    np.random.seed(seed)
    random.seed(seed)

# ✅ GOOD: 디바이스 관리
device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
model = model.to(device)
```

### 체크포인트

```python
# ✅ GOOD: 전체 상태 저장
checkpoint = {
    "epoch": epoch,
    "model_state_dict": model.state_dict(),
    "optimizer_state_dict": optimizer.state_dict(),
    "scheduler_state_dict": scheduler.state_dict(),
    "best_metric": best_metric,
    "config": config,
}
torch.save(checkpoint, f"checkpoints/epoch_{epoch}.pt")

# 복원
checkpoint = torch.load("checkpoints/epoch_10.pt")
model.load_state_dict(checkpoint["model_state_dict"])
optimizer.load_state_dict(checkpoint["optimizer_state_dict"])
```

### 학습 루프

```python
# ✅ GOOD: 구조화된 학습 루프
def train_epoch(model, loader, optimizer, criterion, device):
    model.train()
    total_loss = 0

    for batch in tqdm(loader, desc="Training"):
        inputs = batch["input"].to(device)
        targets = batch["target"].to(device)

        optimizer.zero_grad()
        outputs = model(inputs)
        loss = criterion(outputs, targets)
        loss.backward()

        # Gradient clipping
        torch.nn.utils.clip_grad_norm_(model.parameters(), max_norm=1.0)

        optimizer.step()
        total_loss += loss.item()

    return total_loss / len(loader)
```

---

## 통계 검증

### 모델 비교

```python
from scipy import stats

# ✅ GOOD: 통계적 유의성 검증
def compare_models(scores_a, scores_b, alpha=0.05):
    """두 모델의 cross-validation 점수 비교"""

    # Paired t-test
    t_stat, p_value = stats.ttest_rel(scores_a, scores_b)

    # Effect size (Cohen's d)
    diff = np.array(scores_a) - np.array(scores_b)
    effect_size = diff.mean() / diff.std()

    return {
        "mean_a": np.mean(scores_a),
        "mean_b": np.mean(scores_b),
        "p_value": p_value,
        "significant": p_value < alpha,
        "effect_size": effect_size,
        "better": "A" if np.mean(scores_a) > np.mean(scores_b) else "B",
    }

# 사용
result = compare_models(rf_scores, xgb_scores)
if result["significant"]:
    print(f"Model {result['better']} is significantly better (p={result['p_value']:.4f})")
else:
    print("No significant difference - choose simpler model")
```

### Confidence Interval

```python
from scipy import stats

def confidence_interval(data, confidence=0.95):
    """Bootstrap confidence interval"""
    n = len(data)
    mean = np.mean(data)
    se = stats.sem(data)
    h = se * stats.t.ppf((1 + confidence) / 2, n - 1)
    return mean - h, mean + h

# 사용
accuracy_scores = [0.85, 0.87, 0.84, 0.86, 0.85]
ci_low, ci_high = confidence_interval(accuracy_scores)
print(f"Accuracy: {np.mean(accuracy_scores):.3f} (95% CI: [{ci_low:.3f}, {ci_high:.3f}])")
```

---

## 시각화

### 출판 품질 그래프

```python
import matplotlib.pyplot as plt
import seaborn as sns

# ✅ GOOD: 스타일 설정
plt.style.use("seaborn-v0_8-whitegrid")
plt.rcParams.update({
    "figure.figsize": (10, 6),
    "figure.dpi": 300,
    "font.size": 12,
    "axes.labelsize": 14,
    "axes.titlesize": 16,
    "legend.fontsize": 12,
})

# 저장
fig.savefig("figures/result.png", dpi=300, bbox_inches="tight")
fig.savefig("figures/result.pdf", bbox_inches="tight")  # 벡터
```

### 혼동 행렬

```python
from sklearn.metrics import confusion_matrix, ConfusionMatrixDisplay

# ✅ GOOD: 정규화 + 시각화
cm = confusion_matrix(y_true, y_pred, normalize="true")
disp = ConfusionMatrixDisplay(cm, display_labels=class_names)
disp.plot(cmap="Blues", values_format=".2f")
plt.title("Normalized Confusion Matrix")
plt.savefig("figures/confusion_matrix.png", dpi=300, bbox_inches="tight")
```

---

## 체크리스트

### 실험 시작 전
- [ ] 랜덤 시드 고정
- [ ] 데이터 버전 기록
- [ ] 가설 명시
- [ ] 실험 디렉토리 생성

### 실험 중
- [ ] 파이프라인으로 누수 방지
- [ ] Cross-validation 사용
- [ ] 중간 결과 저장

### 실험 후
- [ ] 통계적 유의성 검증
- [ ] 결과 시각화
- [ ] 재현성 정보 기록
- [ ] 결론 문서화

---

## 프로젝트 구조

```
ml-project/
├── data/
│   ├── raw/              # 원본 데이터 (수정 금지)
│   ├── processed/        # 전처리된 데이터
│   └── versions/         # 데이터 버전 해시
├── experiments/
│   ├── exp001_baseline/
│   ├── exp002_tuning/
│   └── TRACKER.md        # 실험 추적
├── src/
│   ├── data/             # 데이터 로더
│   ├── models/           # 모델 정의
│   ├── training/         # 학습 로직
│   └── evaluation/       # 평가 메트릭
├── notebooks/            # 탐색용 (실험은 scripts)
├── figures/              # 시각화 결과
├── configs/              # 설정 파일
└── tests/                # 테스트
```

---

## 참고

- [K-Dense-AI/claude-scientific-skills](https://github.com/K-Dense-AI/claude-scientific-skills)
- [Reproducible Research](https://www.nature.com/articles/s41562-016-0021)
- [MLOps Principles](https://ml-ops.org/)
