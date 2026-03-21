feature: number-retirement
domain: go-backend
summary: C-넘버 체계 은퇴 — c1/ 삭제, c2/ 분리, 도메인명 전환

requirements:
  ubiquitous:
    - "c1/ 디렉토리는 완전히 삭제되어야 한다"
    - "internal/c2/는 internal/workspace/, internal/persona/, internal/webcontent/ 세 패키지로 분리되어야 한다"
    - "ARCHITECTURE.md는 C-넘버 대신 도메인 맵(Core/Data/Infra/Surface/Doc/Plumbing)을 사용해야 한다"
    - "CLAUDE.md와 SOUL.md에서 C-넘버 표기를 도메인명으로 교체해야 한다"

  event_driven:
    - "WHEN internal/c2/ 분리 시 THEN import path를 참조하는 모든 파일(c2_native.go, store_soul.go)을 업데이트한다"
    - "WHEN c2_native.go 핸들러 분리 시 THEN workspace/persona 각각의 핸들러 파일로 재배치한다"
    - "WHEN webcontent 패키지 승격 시 THEN handlers/webcontent/의 import path도 업데이트한다"

  unwanted:
    - "MCP 도구명(c4_ 접두사)은 변경하지 않는다"
    - "Go 패키지를 역할별 하위 디렉토리로 재그룹하지 않는다 (internal/ 평탄 구조 유지)"
    - "c4-core 리포명은 변경하지 않는다"
    - "docs/ 31개 파일의 C-넘버 일괄 교체는 이 작업에 포함하지 않는다 (점진적)"

non_functional:
  - "go build ./... && go vet ./... 통과 필수"
  - "기존 테스트 전부 통과"

out_of_scope:
  - "docs/ 레거시 문서의 C-넘버 표기 일괄 교체"
  - "MCP 도구명 변경"
  - "c4-core 리포명 변경"
  - "internal/ 하위 디렉토리 재구조화"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/workspace/... ./internal/persona/... ./internal/webcontent/..."
    - "test ! -d c1/"
    - "test ! -d c4-core/internal/c2/"
