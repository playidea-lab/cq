# C9 Survey — HMR Attention Mechanism & Transformers 2024-2025
Date: 2026-02-28
Round: 1 (Round 1 훈련 대기 중 사전 조사)

### Key Papers (관련도 순)
| # | Title | Year | arXiv | Key Claim |
|---|-------|------|-------|-----------|
| 1 | **GenHMR**: Generative Human Mesh Recovery | 2025 | [2412.14444](https://arxiv.org/abs/2412.14444) | Pose를 discrete token으로 변환, masked transformer로 SOTA 달성 |
| 2 | **PromptHMR**: Promptable Human Mesh Recovery | 2025 | [2504.06397](https://arxiv.org/abs/2504.06397) | 멀티모달 prompt를 cross-attention에 주입, depth/occlusion 해결 |
| 3 | **DeforHMR**: ViT with Deformable Cross-Attention | 2025 | [2411.11214](https://arxiv.org/abs/2411.11214) | frozen ViT encoder + deformable attention으로 sparse feature 집중 |
| 4 | **HSMR**: Biomechanically Accurate HMR | 2025 | [2408.10221](https://arxiv.org/abs/2408.10221) | SKEL 생체역학 제약을 transformer에 통합 |
| 5 | **POTTER**: Pooling Attention Transformer | 2024 | [2403.09063](https://arxiv.org/abs/2403.09063) | Pooling attention으로 파라미터 93% 감소 |
| 6 | **TokenHMR** | 2024 | [2312.00138](https://arxiv.org/abs/2312.00138) | TALS + discrete token으로 alignment 개선 |

### SOTA Results (3DPW / Human3.6M)
| Method | Dataset | Metric | Score |
|--------|---------|--------|-------|
| GenHMR | Human3.6M | MPJPE | **33.5mm** |
| CLIFF+KITRO | 3DPW | PA-MPJPE | **27.67mm** |
| DeforHMR | RICH | MPJPE | **68.2mm** |
| HMR2.0 | 3DPW | PA-MPJPE | 44.4mm |
| **우리 baseline** | 3DPW | MPJPE | **102.6mm** |

### Critical Findings
- **지배적 트렌드**: CNN 회귀 → **Generative Pose Tokenization** (GenHMR, TokenHMR). 우리 SimVQ 방향과 정렬됨
- **Frozen ViT vs Fine-tuning 논쟁**: frozen encoder는 spatial localization이 약해 deformable attention 필요
- **실패 모드**: Depth ambiguity, inter-person overlap
- **우리 연구 갭**: VQ codebook이 체계적인 "pose vocabulary"로 작동하는지 검증 필요

### C9 Conference Input (Round 2용)
> 2025 HMR 문헌에서 **Generative Pose Tokenization**이 지배적 방향. 우리 SimVQ latent가
> 의미있는 pose vocabulary를 학습하는지 확인 후, Round 2에서는 Deformable Attention 디코더
> (DeforHMR 스타일) 또는 discrete token 활용 방식(GenHMR 스타일)으로 가설 수립 권장.
