---
name: model-serving
description: |
  ML 모델 서빙 가이드. ONNX 변환, API 래핑, A/B 테스트, 카나리 배포, 모니터링까지
  프로덕션 서빙 전 과정을 안내합니다. 학습된 모델을 배포하거나 서빙 최적화가 필요할 때
  반드시 이 스킬을 사용하세요. "모델 서빙", "model serving", "모델 배포", "ONNX 변환",
  "추론 API", "inference", "모델 최적화" 등의 요청에 트리거됩니다.
---

# Model Serving

ML 모델 서빙 가이드.

## 트리거

"모델 서빙", "model serving", "모델 배포", "ONNX 변환", "추론 API", "inference"

## Steps

### 1. 모델 변환

학습 코드(raw PyTorch/TF)를 직접 서빙하지 않는다:

```python
# PyTorch → ONNX
import torch
dummy_input = torch.randn(1, 3, 224, 224)
torch.onnx.export(model, dummy_input, "model.onnx",
                  opset_version=17,
                  input_names=['input'],
                  output_names=['output'],
                  dynamic_axes={'input': {0: 'batch'}, 'output': {0: 'batch'}})
```

| 형식 | 장점 | 적합한 경우 |
|------|------|------------|
| ONNX | 범용, 최적화 도구 풍부 | 대부분 |
| TorchScript | PyTorch 생태계 유지 | PyTorch 전용 인프라 |
| TensorRT | NVIDIA GPU 최적화 | 지연 민감 서비스 |
| ONNX + quantization | 크기 축소 | 엣지/모바일 |

### 2. API 래핑

```python
# FastAPI 예시
from fastapi import FastAPI, UploadFile
import onnxruntime as ort

app = FastAPI()
session = ort.InferenceSession("model.onnx")

@app.post("/predict")
async def predict(file: UploadFile):
    data = preprocess(await file.read())
    result = session.run(None, {"input": data})
    return {"prediction": postprocess(result)}

@app.get("/health")
async def health():
    return {"status": "ok", "model_version": "v1.2.0"}
```

**필수 엔드포인트:**
- `POST /predict` — 추론
- `GET /health` — 헬스체크
- `GET /model/info` — 모델 버전, 메트릭

### 3. 성능 최적화

| 기법 | 효과 | 비고 |
|------|------|------|
| 배치 처리 | 처리량 향상 | dynamic batching |
| 양자화 (INT8) | 2-4x 속도, 크기 절반 | 정확도 1-2% 감소 가능 |
| 모델 프루닝 | 크기 축소 | 재학습 필요할 수 있음 |
| GPU 추론 | 10-100x 속도 | 비용 고려 |
| 캐싱 | 중복 요청 제거 | 동일 입력 빈번할 때 |

### 4. A/B 테스트 / 카나리 배포

```
v1 (현재) ─── 90% 트래픽
                              ─── 로드밸런서 ─── 사용자
v2 (신규) ─── 10% 트래픽
```

- 카나리: 10% → 50% → 100% (문제 없으면 점진 확대)
- 롤백 기준: 지연 2x 이상 또는 에러율 1% 이상
- 1-command 롤백 가능해야 함

### 5. 모니터링

| 지표 | 알림 기준 |
|------|----------|
| 추론 지연 (P95) | > 임계값 |
| 에러율 | > 1% |
| GPU 메모리 | > 90% |
| 입력 분포 (drift) | PSI > 0.2 |
| 예측 분포 | 급격한 변화 |

```python
# PSI (Population Stability Index) 기반 드리프트 감지
def psi(expected, actual, bins=10):
    e_pct = np.histogram(expected, bins=bins)[0] / len(expected)
    a_pct = np.histogram(actual, bins=bins)[0] / len(actual)
    return np.sum((a_pct - e_pct) * np.log((a_pct + 1e-6) / (e_pct + 1e-6)))
```

### 6. 체크리스트

- [ ] 모델 변환 완료 (ONNX/TorchScript)
- [ ] 변환 전후 정확도 일치 확인
- [ ] API health check 엔드포인트 동작
- [ ] 부하 테스트 완료 (예상 트래픽 2배)
- [ ] 모니터링 대시보드 설정
- [ ] 롤백 절차 준비 및 테스트
- [ ] 모델 레지스트리에 버전 등록

## 안티패턴

- 학습 코드 그대로 서빙 (raw PyTorch `.py` 파일)
- 모니터링 없이 배포 ("잘 돌아가겠지")
- 롤백 계획 없음
- 모델 버전 관리 없음 (어떤 모델이 서빙 중인지 모름)
