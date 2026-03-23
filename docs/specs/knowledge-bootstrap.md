feature: knowledge-bootstrap
domain: go-backend
description: 논문/URL에서 LLM으로 교훈을 추출하여 paper-lesson으로 저장하고, get_task 시 관련 교훈을 자동 주입하는 파이프라인.

requirements:
  - type: event-driven
    text: "When 사용자가 논문 URL 또는 텍스트를 제공하면, the system shall LLM으로 교훈을 추출하여 paper-lesson doc_type으로 knowledge에 저장한다"
  - type: event-driven
    text: "When get_task로 태스크를 할당할 때, the system shall 태스크 scope/domain/tags와 매칭되는 paper-lesson을 knowledge_context에 주입한다"
  - type: state-driven
    text: "While paper-lesson이 없으면, the system shall 기존 knowledge_context만 제공한다"
  - type: optional
    text: "If 논문이 PDF이면, the system shall 텍스트를 추출하여 LLM에 전달한다"
  - type: unwanted
    text: "If LLM 추출이 실패하면, the system shall 에러를 반환하고 기존 knowledge에 영향을 주지 않는다"

non_functional:
  - 교훈 추출은 비동기 불필요 (사용자 명시 호출)
  - topK=3 제한으로 paper-lesson 과잉 주입 방지

out_of_scope:
  - L3 집단 패턴 수집/공유
  - 유효성 피드백 자동 추적 (Stage 2)
  - 논문 자동 검색/추천