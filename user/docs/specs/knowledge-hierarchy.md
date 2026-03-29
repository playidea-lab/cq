feature: knowledge-hierarchy
domain: go-backend
summary: 4계층 지식 아키텍처 — Project→User→Team→Community. Layer 2(글로벌)부터 구현.

requirements:
  ubiquitous:
    - "c4_knowledge_record에 scope 옵션(project|global)이 있어야 한다"
    - "scope=global이면 ~/.c4/knowledge.db에도 저장해야 한다"
    - "c4_persona_learn 완료 시 ~/.c4/personas/에 delta 병합해야 한다"
    - "프로젝트 시작 시 ~/.c4/knowledge.db에서 관련 지식을 자동 로드해야 한다"

  event_driven:
    - "WHEN c4_knowledge_record(scope=global) THEN 프로젝트 DB + 글로벌 DB 양쪽 저장"
    - "WHEN cq claude 실행 시 THEN ~/.c4/knowledge.db에서 domain 매칭 지식 프로젝트에 주입"
    - "WHEN c4_persona_learn_from_diff 완료 THEN ~/.c4/personas/에 통합 패턴 병합"

  unwanted:
    - "글로벌 지식 DB 접근 실패가 프로젝트 동작을 방해하면 안 된다"
    - "글로벌 persona가 프로젝트 고유 패턴을 덮어쓰면 안 된다"

  optional:
    - "IF community.contribute=true THEN 익명화된 패턴을 커뮤니티 풀에 자동 제출"

out_of_scope:
  - "Layer 4 Community aggregator (설계만)"
  - "Hub knowledge 테이블 마이그레이션 (Layer 3은 기존 publish/pull 유지)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/knowledge/..."
