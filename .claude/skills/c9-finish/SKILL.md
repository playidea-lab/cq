---
name: c9-finish
description: |
  C9 연구 루프 FINISH phase. 최고 모델 저장, 결과 문서화, 아티팩트 정리.
  트리거: "c9-finish", "연구 완료", "연구 마무리", "finish research"
allowed-tools: Bash, Read, mcp__cq__*
---
# C9 Finish

연구 루프 완료 시 best model 저장 + 결과 문서화.
배포(/c9-deploy)는 별도 실행.

**트리거**: "c9-finish", "연구 마무리", "결과 정리", "finish"

## 실행 순서

### Step 1: Best model 확인
```bash
cat .c9/state.yaml  # mpjpe_history에서 best round/exp 확인
```

### Step 2: Best checkpoint 저장 + Drive 업로드
```bash
# C5 Hub Job으로 pi 서버에서 best model 복사
cq hub submit --run "cp /home/pi/git/hmr_unified/experiments/paper1/BEST_EXP/best_checkpoint.pt /home/pi/git/hmr_unified/outputs/c9_best_model.pt && echo SAVED"
```

Best model을 Drive에 업로드 (다른 워커/세션에서 재사용 가능):
```
c4_drive_upload(
  local_path: "<best_model_local_path>",
  drive_path: "/c9/{project_name}/models/best_model.pt",
  metadata: {"mpjpe": best_mpjpe, "round": best_round, "exp": best_exp_name}
)
```

### Step 2.5: 실험 디렉토리 스냅샷 (Dataset)

전체 실험 디렉토리를 버전 관리하여 재현 가능하게 보존:
```
c4_drive_dataset_upload(
  path: "<experiments_dir>",
  name: "c9-{project_name}-experiments"
)
```
→ 동일 해시면 스킵 (변경분만 업로드). 나중에 `c4_drive_dataset_pull(name)` 으로 복원.

### Step 3: 결과 문서 생성
`.c9/RESEARCH_SUMMARY.md` 생성:
```markdown
# C9 Research Summary — [날짜]

## 최종 결과
- Best MPJPE: Xmm (Round N, exp_name)
- Baseline: 102.6mm
- 총 개선: X.Xmm (X.X%)

## 라운드별 진행
| Round | 핵심 실험 | MPJPE | 가설 |

## 핵심 발견
- ...

## 다음 연구 방향
- ...
```

### Step 3.5: Knowledge Recording (finish 패턴)

연구 루프 전체 결과를 Knowledge DB에 기록.

#### 3.5.1 실험 결과 기록

mpjpe_history의 각 라운드 best 결과를 기록 (Drive 경로 포함):

```
c4_experiment_record(
  title: "R{N} {exp_name}: MPJPE={X}mm",
  content: |
    Round: {N}, Config: {요약}
    MPJPE: {X}mm, PA: {Y}mm, Util: {Z}%
    vs baseline: {diff}mm ({%}%)
    Artifacts:
      - Model: /c9/{project_name}/models/best_model.pt (c4_drive_download)
      - Experiments: c9-{project_name}-experiments (c4_drive_dataset_pull)
  tags: ["c9", "round-{N}", "{exp_name}"]
)
```

#### 3.5.2 연구 패턴 기록

전체 루프에서 발견된 성공/실패 패턴:

```
c4_knowledge_record(
  doc_type: "pattern",
  title: "Research: {주제} — {핵심 결론}",
  content: |
    ## 결과: Best MPJPE {X}mm (개선 {Y}mm, {N}라운드)

    ## 성공 패턴
    - {효과 있었던 접근}

    ## 실패 패턴
    - {효과 없었던 접근}

    ## 재사용 인사이트
    - {다음 연구에 적용 가능한 교훈}
  tags: ["research", "pattern", "{주제}"],
  visibility: "team"
)
```

#### 3.5.3 합의문 기록

각 라운드 conference 합의문 중 핵심만 기록:

```
c4_knowledge_record(
  doc_type: "hypothesis",
  title: "Hypothesis: {연구 주제} R{N} — {합의 1줄}",
  content: {합의문 핵심},
  tags: ["conference", "{주제}"]
)
```

### Step 4: state.yaml 업데이트
```yaml
phase: DONE
finish:
  best_round: N
  best_mpjpe: X.X
  best_model_path: /home/pi/.../c9_best_model.pt
  completed_at: ISO8601
```

### Step 5: git commit (선택)
```bash
git add .c9/ && git commit -m "c9-finish: round N best MPJPE=Xmm"
```

## 완료 후 선택지
- `/c9-deploy` → edge 서버에 배포
- `/c9-conference` → 다음 연구 사이클 시작 (새 가설)
- 루프 종료 (state=DONE)
