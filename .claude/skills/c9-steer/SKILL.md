# C9 Steer

state.yaml을 직접 편집하지 않고 phase 전환과 reason 업데이트를 추상화하는 조종 스킬.
partial write 방지를 위해 원자 저장 패턴(NamedTemporaryFile → os.replace)을 사용한다.

**트리거**: "c9-steer", "/c9-steer", "방향 바꿔", "phase 전환", "스티어"

## 사용법

```
/c9-steer "방향 설명"   → reason 업데이트 + phase 전환 (현재 phase 규칙 적용)
/c9-steer --status      → 현재 state.yaml 내용 출력
/c9-steer --reset       → phase=CONFERENCE, round=1 (FINISH 상태에서만 허용)
/c9-steer --skip        → 현재 phase 건너뜀 (다음 phase로 강제 전환)
/c9-steer --back        → 이전 phase로 복귀
```

## Phase 전이 테이블

| 현재 Phase | `/c9-steer "방향"` | `--skip` | `--back` |
|------------|-------------------|----------|----------|
| CONFERENCE | reason 업데이트 | — | (round=1이면 경계 안내) |
| IMPLEMENT | reason 업데이트 | → RUN | → CONFERENCE |
| RUN | reason 업데이트 (경고 포함) | → CHECK | → IMPLEMENT |
| CHECK | reason 업데이트 | → REFINE | → RUN |
| REFINE | reason 업데이트 + → CONFERENCE | — | → CHECK |
| FINISH | — | — | → REFINE |
| DONE | — | — | → FINISH |

- **CONFERENCE + "방향"**: reason만 업데이트, phase 변경 없음 (다음 conference 토론 방향 설정)
- **REFINE + "방향"**: reason 업데이트 후 phase=CONFERENCE 자동 전환 (재설계 트리거)
- **RUN + "방향"**: active_jobs가 있으면 경고 출력 후 reason만 업데이트

## 실행 순서

### Step 1: 현재 state 읽기
```bash
cat .c9/state.yaml
```
phase, round, active_jobs, reason(있으면) 확인.

### Step 2: active_jobs 경고 체크
active_jobs 목록이 비어있지 않으면:

```
⚠️  active_jobs가 있습니다: [job-xxx, job-yyy]
    실행 중인 작업이 있는 상태에서 phase를 전환하면 결과가 유실될 수 있습니다.
    계속 진행하시겠습니까? (yes/no)
```

→ 사용자가 "yes" 응답 시에만 진행. "no"이면 중단.

### Step 3: 명령 분기

**`--status`**:
```bash
cat .c9/state.yaml
```
현재 state를 그대로 출력하고 종료.

**`--reset`** (FINISH 또는 DONE 상태에서만):
- FINISH/DONE이 아닌 경우: `"현재 phase={phase}입니다. --reset은 FINISH/DONE 상태에서만 허용됩니다."` 출력 후 종료.
- FINISH/DONE인 경우: phase=CONFERENCE, round=1로 설정.

**`--skip`**:

| 현재 phase | 전환 대상 | 비고 |
|-----------|-----------|------|
| IMPLEMENT | RUN | |
| RUN | CHECK | active_jobs 경고 필수 |
| CHECK | REFINE | |
| REFINE | CONFERENCE | |
| CONFERENCE | — | "CONFERENCE는 --skip 미지원 (토론 먼저)" 안내 |
| FINISH | — | "--skip은 FINISH에서 지원하지 않습니다. --reset 사용" 안내 |

**`--back`**:

| 현재 phase | 복귀 대상 | 비고 |
|-----------|-----------|------|
| IMPLEMENT | CONFERENCE | |
| RUN | IMPLEMENT | active_jobs 경고 필수 |
| CHECK | RUN | |
| REFINE | CHECK | |
| FINISH | REFINE | |
| DONE | FINISH | |
| CONFERENCE (round=1) | — | "round=1 경계: 더 이전 phase가 없습니다" 안내 |
| CONFERENCE (round>1) | REFINE | 이전 round의 REFINE으로 복귀 개념 |

**`"방향"` (reason 업데이트)**:

phase별 동작:
- **CONFERENCE**: reason 업데이트만 (phase 유지)
- **IMPLEMENT**: reason 업데이트만 (phase 유지)
- **RUN**: active_jobs 있으면 경고 포함, reason 업데이트만 (phase 유지)
- **CHECK**: reason 업데이트만 (phase 유지)
- **REFINE**: reason 업데이트 + phase=CONFERENCE 자동 전환
- **FINISH/DONE**: `"현재 phase={phase}입니다. --reset 또는 --back을 사용하세요."` 안내

### Step 4: 원자 저장 (partial write 방지)

```python
import yaml
import os
import tempfile

# 현재 state 읽기
with open(".c9/state.yaml", "r") as f:
    state = yaml.safe_load(f)

# 변경 적용
state["phase"] = new_phase         # phase 전환 시
state["steer_reason"] = reason     # reason 업데이트 시
state["round"] = new_round         # round 변경 시

# 원자 저장: NamedTemporaryFile → os.replace
with tempfile.NamedTemporaryFile(
    mode="w",
    dir=".c9",
    suffix=".yaml.tmp",
    delete=False
) as tmp:
    yaml.dump(state, tmp, allow_unicode=True, default_flow_style=False)
    tmp_path = tmp.name

os.replace(tmp_path, ".c9/state.yaml")
```

또는 bash inline (python3 사용):

```bash
python3 - <<'EOF'
import yaml, os, tempfile

with open(".c9/state.yaml") as f:
    state = yaml.safe_load(f)

state["phase"] = "CONFERENCE"
state["steer_reason"] = "사용자 지정 방향"

with tempfile.NamedTemporaryFile(mode="w", dir=".c9", suffix=".yaml.tmp", delete=False) as tmp:
    yaml.dump(state, tmp, allow_unicode=True, default_flow_style=False)
    tmp_path = tmp.name

os.replace(tmp_path, ".c9/state.yaml")
print("저장 완료")
EOF
```

### Step 5: 결과 보고

```
## C9 Steer 결과

이전 state: phase=REFINE, round=2
변경 사항:
  - phase: REFINE → CONFERENCE
  - steer_reason: "SimVQ quantization 대신 VQ-VAE 기반 재실험 방향"

현재 state: phase=CONFERENCE, round=2
다음 단계: /c9-conference 또는 /c9-loop
```

## 주의사항

- **state.yaml 직접 편집 금지**: 이 스킬을 통해서만 변경한다. 직접 편집 시 partial write 위험.
- **active_jobs 있을 때 --skip/--back**: 경고 후 사용자 확인 필수. 자동으로 진행하지 않는다.
- **round=1에서 --back**: 더 이전 phase가 없음을 안내하고 중단.
- **IMPLEMENT phase 없는 경우**: 일부 실험은 CONFERENCE → RUN으로 바로 전환됨. --back 시 CONFERENCE로 복귀.
- **yaml 라이브러리 없는 환경**: `python3 -c "import yaml"` 먼저 확인. 없으면 `pip3 install pyyaml` 또는 `uv add pyyaml`.

## 예시

```bash
# 현재 상태 확인
/c9-steer --status

# REFINE 중 방향 수정 (자동으로 CONFERENCE 전환)
/c9-steer "SimVQ 대신 RVQ(Residual VQ) 기반으로 방향 전환"

# IMPLEMENT 건너뛰기 (구현 없이 바로 RUN)
/c9-steer --skip

# RUN에서 CHECK로 강제 이동 (실험 결과가 이미 있을 때)
/c9-steer --skip

# 이전 단계로 복귀
/c9-steer --back

# 연구 완료 후 새 사이클 시작
/c9-steer --reset
```
