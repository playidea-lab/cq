---
name: paper-writing
description: |
  학술 논문 작성 가이드. 논문 구조, Abstract, Introduction, Method, Experiment, Conclusion
  각 섹션 작성법을 체계적으로 가이드합니다. "논문 작성", "paper writing", "학술 논문 쓰기",
  "논문 초안", "manuscript", "논문 구조" 등의 요청에 트리거됩니다.
---
# Paper Writing

학술 논문 작성 가이드.

## 트리거

"논문 작성", "paper writing", "학술 논문 쓰기", "논문 초안", "manuscript"

## Steps

### 1. 구조 설계

논문 작성 전 뼈대를 잡는다:

```
1. Title — 핵심 기여를 한 문장으로
2. Abstract — 문제/방법/결과/의의 (150-250단어)
3. Introduction — 문제 정의, 기존 연구 한계, 우리 기여
4. Related Work — 관련 연구 분류 + 차별점
5. Method — 제안 방법 상세
6. Experiments — 설정, 결과, 분석
7. Discussion — 한계, 의의, 미래 연구
8. Conclusion — 요약 (Abstract와 다르게)
```

### 2. Introduction 작성법

4단락 구조:

1. **문제와 맥락**: 왜 이 문제가 중요한가? (넓은 맥락 → 구체적 문제)
2. **기존 연구 한계**: 현재 접근법의 문제점 (2-3개 구체적으로)
3. **우리 기여**: "In this paper, we propose..." — 핵심 아이디어와 차별점
4. **결과 요약 + 논문 구조**: "Our experiments show..." + "The rest of this paper..."

### 3. Method 작성법

- 전체 파이프라인을 그림 1개로 (Figure 1은 항상 전체 구조도)
- 수식은 정의 → 유도 → 직관적 설명 순서
- 모든 기호는 첫 등장 시 정의
- 복잡한 방법은 서브섹션으로 분할

### 4. Experiments 작성법

**필수 항목:**

| 항목 | 설명 |
|------|------|
| 데이터셋 | 이름, 크기, 분할(train/val/test), 전처리 |
| 베이스라인 | 비교 대상 + 선정 이유 |
| 메트릭 | 평가 지표 + 선정 이유 |
| 구현 | 프레임워크, 하이퍼파라미터, 학습 설정 |
| 결과 | mean ± std (3회+), 굵은 글씨 = 최고 성능 |
| 어블레이션 | 각 컴포넌트의 기여 분리 검증 |

**표/그래프 규칙:**
- 캡션만으로 이해 가능해야 한다
- 300 DPI 이상
- 축 레이블 + 단위 필수
- 색상은 색각 이상자 고려 (colorblind-safe palette)

### 5. 작성 원칙

- **한 문단 = 한 아이디어**: 첫 문장이 요약, 나머지가 뒷받침
- **능동태 우선**: "We propose X" > "X is proposed"
- **짧은 문장**: 한 문장에 정보 1-2개. 3개 이상이면 분할
- **반복 피하기**: Abstract, Intro, Conclusion에서 같은 문장 복붙 금지
- **구체적으로**: "significantly better" → "3.2% improvement (p < 0.01)"

### 6. 제출 전 체크리스트

- [ ] 모든 그림/표가 본문에서 참조되는가?
- [ ] 모든 수식 기호가 정의되어 있는가?
- [ ] Abstract가 독립적으로 이해 가능한가?
- [ ] 참고 문헌 형식이 일관적인가?
- [ ] 저자 정보, 감사의 글 확인
- [ ] 페이지 제한 준수
- [ ] 영어 문법 검토 (non-native라면 Grammarly/동료 검토)
- [ ] 보충 자료 (supplementary) 포함 여부

## CQ 연동 (CQ 프로젝트인 경우)

| 작업 | CQ 도구 |
|------|--------|
| 논문 자동 리뷰 | `/c4-review` (3-pass + persona learning) |
| 실험 결과 기록 | `c4_experiment_record` |
| 문헌 조사 | `/c9-survey` |
| 실험-리뷰 반복 | `/research-loop` |

## 안티패턴

- 실험 결과 없이 방법론만 쓰기
- 베이스라인 없는 결과 제시
- "future work"에 핵심 한계를 숨기기
- 그래프에 축 레이블/단위 없음
- 결론에서 과장 ("revolutionary", "groundbreaking")
