
## Round 1 — 2026-02-28

- **가설**: STE → SimVQ (linear reparameterization)로 codebook collapse 해결 후 MPJPE 개선
- **실험**: exp_simvq (VQ-VAE 재훈련 + HMR 30 epochs)
- **결과**: MPJPE=102.89mm, PA-MPJPE=72.43mm, codebook_util=?
- **개선**: -0.29mm (baseline 102.6mm 대비 **악화**)
- **핵심 발견**:
  - SimVQ의 linear basis reparameterization이 codebook collapse를 완화하더라도 HMR MPJPE에 직접적 영향 없음
  - AUX 잠재벡터 차원이 486dim으로 baseline 대비 동일하지만 representation quality 자체가 bottleneck일 가능성
  - eval_exp056.py 별도 실행 필요 (training script에서 MPJPE 직접 출력 안 함) — 이는 파이프라인 버그
- **다음 방향**:
  1. Codebook 개선보다 **attention 메커니즘** 자체 개선 (survey: GenHMR 33.5mm, attention에 geometric prior)
  2. Aux latent의 **차원 축소** (486→128, PCA 아닌 학습된 projection)
  3. **KD loss weight** 조정 (현재 aux 정보가 HMR training에 얼마나 기여하는지 ablation)
