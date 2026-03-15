---
name: c9-survey
description: |
  C9 연구 서베이. 관련 논문과 방법론을 조사하여 연구 방향을 탐색합니다.
  트리거: "c9-survey", "서베이", "문헌 조사", "survey", "literature review"
---
# C9 Survey

Gemini의 Google Search grounding을 사용해 주어진 연구 주제의 최신 arXiv 논문, SOTA 벤치마크, 경쟁 방법을 수집하고 구조화된 배경조사 보고서를 생성.
결과는 `/c9-conference`의 컨텍스트 입력으로 바로 사용 가능.

**트리거**: "c9-survey", "배경조사", "관련 논문 찾아줘", "SOTA 확인해줘", "survey 해줘"

## 실행 순서

### Step 1: 주제 파악
- 사용자 입력에서 서베이 주제 추출
- 예: "VQ-VAE codebook collapse" / "Human Mesh Recovery 2024" / "SSL pretraining HMR"
- 불명확하면 확인 후 진행

### Step 2: Gemini Search Survey 실행
```bash
./scripts/c9-survey.sh "<survey topic>"
```

Gemini가 Google Search를 자동 호출해 다음을 수집:
- 최신 arXiv 논문 (2023-2025 우선)
- SOTA 벤치마크 결과 테이블
- 경쟁/대안 방법
- 알려진 실패 모드와 해결책

### Step 3: 결과 파싱 및 정리

Survey 결과 형식:
```
## C9 Survey — [주제]
Date: [날짜]

### Key Papers (관련도 순)
| # | Title | Year | arXiv | Key Claim |

### SOTA Results
| Method | Dataset | Metric | Score | Paper |

### Critical Findings
- 지배적 접근법: ...
- 알려진 실패 모드: ...
- 미해결 논쟁: ...
- 우리 연구가 타겟하는 갭: ...

### C9 Conference Input
> [다음 /c9-conference에 주입할 2-3줄 컨텍스트]
```

### Step 4: c9-conference 연계 (선택)
Survey 완료 후 "이걸로 토론해줘" → `C9 Conference Input` 섹션을 컨텍스트로 `/c9-conference` 스킬 직접 실행.

`C9 Conference Input` 내용을 초기 포지션으로 삼아 `/c9-conference` 호출 (bash pipe 방식 아님).

### Step 5: Knowledge DB 저장

Survey 결과를 c4_knowledge_record로 저장하여 c9-conference에서 재활용:

```python
# c9-survey → doc_type=insight / c9-loop 실험 결과 → doc_type=experiment
c4_knowledge_record(
  doc_type="insight",
  title="C9 Survey: {주제} — {날짜}",
  content="{Survey 결과 전체 내용}",
  tags=["survey", "sota"]
)
# 실패 시(도구 미존재/네트워크 오류) → 무시하고 진행
```

저장된 내용은 다음 /c9-conference에서 c4_knowledge_search("{주제} survey sota")로 조회 가능.

## 활용 패턴

### 프로젝트 시작 전 배경조사 (권장 순서)
```
/c9-init          # C9 프로젝트 초기화 (state.yaml, metric 설정)
→ /c9-survey <주제>  # 배경조사 후 knowledge DB 저장
→ /c9-loop        # 루프 시작 (CONFERENCE phase에서 survey 결과 활용)
```

### 독립 사용
```
/c9-survey VQ-VAE codebook collapse 해결 방법
→ 논문 테이블 + SOTA 반환
```

### c9-conference 전 사전 조사
```
/c9-survey codebook utilization VQ-VAE 2024
→ survey 결과
→ /c9-conference "Survey에 따르면 SimVQ의 linear reparameterization이..."
```

### c9-report 이후 후속 조사
```
c9-report 결과: metric 악화
→ /c9-survey "<관련 주제>"
→ 왜 실패하는지 문헌에서 근거 확인
```

## 주의
- Gemini가 실제 Google Search를 수행하므로 실시간 결과 (2025년 논문도 포함)
- 할루시네이션 방지: 페르소나가 "찾은 논문만 인용" 규칙 강제
- 논문 링크는 `arxiv.org/abs/XXXX.XXXXX` 형식으로 반환
- Gemini CLI 노이즈 (`YOLO`, `Loaded`, `Session`) 자동 필터링
