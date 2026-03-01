---
description: |
  C9 연구 루프를 위한 새 프로젝트를 초기화합니다. state.yaml과 필수 디렉토리를 생성하고,
  Hub URL, 메트릭, 수렴 조건 등을 대화형으로 설정합니다.
  트리거: "c9-init", "c9 초기화", "연구 루프 초기화", "c9 시작", "init c9"
---

# C9 Init — 연구 루프 초기화

신규 C9 프로젝트의 진입점. `.c9/state.yaml`과 필수 디렉토리를 생성합니다.

**트리거**: "c9-init", "c9 초기화", "연구 루프 초기화", "c9 시작"

## 실행 순서

### Step 1: 기존 .c9/ 존재 여부 확인

```bash
ls .c9/ 2>/dev/null && echo "EXISTS" || echo "NEW"
```

기존 `.c9/` 디렉토리가 존재하면:
- 사용자에게 덮어쓰기 여부를 질문 (AskUserQuestion)
  - "기존 .c9/ 디렉토리가 발견되었습니다. 덮어쓰시겠습니까? (yes/no)"
  - "no" → 중단하고 기존 state.yaml 경로 안내 후 종료
  - "yes" → 계속 진행

기존 `.c9/state.yaml`이 존재하면:
- 마이그레이션 안내 출력:
  ```
  ⚠️  기존 state.yaml이 발견되었습니다.
  새 스키마 형식은 docs/c9-state-schema.md를 참고하세요.
  마이그레이션(migration)이 필요한 경우 백업 후 진행을 권장합니다.
  ```

### Step 2: Q1 — 프로젝트명 + 연구 목표 (AskUserQuestion)

질문:
```
프로젝트명과 연구 목표를 입력해주세요.
예시:
  프로젝트명: unified-hmr
  연구 목표: 3D Human Mesh Recovery 정확도 향상 (MPJPE 최소화)
```

입력받은 값을 `project_name`, `research_goal` 변수에 저장.

### Step 3: Q2 — Hub URL (AskUserQuestion)

질문:
```
C5 Hub URL을 입력해주세요. (https:// 형식)
예시: https://your-c5-hub.fly.dev
```

검증:
- `https://`로 시작하는지 확인
- 유효하지 않으면 재요청 (retry): "URL은 https:// 형식이어야 합니다. 다시 입력해주세요."
- 유효하면 `hub_url` 변수에 저장

### Step 4: Q3 — 메트릭 설정 (AskUserQuestion)

질문 (한 번에):
```
실험 메트릭을 설정합니다.

1. 메트릭 이름 (예: MPJPE, F1, BLEU, accuracy 등):
2. 낮을수록 좋은 메트릭입니까? (lower_is_better: true/false):
3. 베이스라인 값 (float, 예: 102.6):
4. 수렴 임계값 (convergence_threshold, float, 예: 0.5):
   → 이 값 이상 개선되지 않으면 수렴으로 판단합니다.
```

검증:
- `baseline` 값이 float으로 파싱되는지 확인
  - 유효하지 않으면 재요청 (retry): "베이스라인 값은 숫자(float)여야 합니다. 다시 입력해주세요."
- `convergence_threshold` 값이 float으로 파싱되는지 확인
  - 유효하지 않으면 재요청 (retry): "수렴 임계값은 숫자(float)여야 합니다. 다시 입력해주세요."
- `lower_is_better`가 true/false인지 확인
  - 유효하지 않으면 재요청 (retry): "lower_is_better는 true 또는 false여야 합니다."

### Step 5: 디렉토리 및 state.yaml 생성

```bash
mkdir -p .c9/experiments
mkdir -p .c9/rounds
```

(mkdir -p로 멱등 처리 — 이미 존재해도 오류 없음)

`.c9/state.yaml` 생성 (아래 템플릿 사용):

```yaml
# C9 Research Loop State
# phase: CONFERENCE | IMPLEMENT | RUN | CHECK | REFINE | FINISH | DONE
phase: CONFERENCE
round: 1

project:
  name: "<project_name>"
  goal: "<research_goal>"

hub:
  url: "<hub_url>"
  # API Key는 별도 저장: cq secret set c9.hub.api_key <your-key>

metric:
  name: "<metric_name>"
  lower_is_better: <lower_is_better>
  baseline: <baseline>
convergence_threshold: <convergence_threshold>
max_rounds: 10

last_check: ~

notify:
  dooray_webhook: ""
  session: ""
  bot_name: "C9 Lab"
  server_id: ""
  templates:
    dooray: "{emoji} **[C9 R{round} · {phase}]** [{server}] {message}"
    mail: "[C9-{phase}] Round={round} server={server} {message}"

active_jobs: []

mpjpe_history:
  - round: 0
    best_mpjpe: <baseline>
    note: "baseline"

finish:
  best_round: ~
  best_mpjpe: ~
  best_model_path: ~
  artifact_path: ~
  completed_at: ~
```

### Step 6: 완료 안내 출력

```
✅ C9 초기화 완료!

생성된 파일/디렉토리:
  .c9/state.yaml        ← 연구 루프 상태
  .c9/experiments/      ← 실험 yaml 저장 위치
  .c9/rounds/           ← 라운드별 결과 저장 위치

다음 단계:
  1. Hub API Key 저장 (직접 터미널에서 실행):
     cq secret set c9.hub.api_key <your-key>

  2. 연구 루프 시작:
     /c9-loop  또는  /c9-conference

설정 확인:
  cat .c9/state.yaml
```

> **보안**: API Key는 이 스킬에서 직접 실행하지 않습니다.
> 사용자가 터미널에서 직접 `cq secret set c9.hub.api_key <your-key>`를 실행해야 합니다.

## 주의사항

- 기존 `.c9/`가 있을 때 덮어쓰기(overwrite) 전 반드시 확인 질문
- `docs/c9-state-schema.md`의 마이그레이션(migration) 가이드 참조 안내
- float 검증 실패 시 해당 필드만 재요청 (retry), 전체 처음부터 다시 하지 않음
- Hub URL은 `https://`로 시작해야 하며, 유효하지 않으면 재요청

## 사용 예시

```text
/c9-init
```
