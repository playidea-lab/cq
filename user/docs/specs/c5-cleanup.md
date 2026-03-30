feature: C5 Dead Code 정리 + 빌드 태그 정규화
domain: web-backend
requirements:
  ubiquitous:
    - c5_hub 빌드 태그는 hub로 rename되어야 한다 (24개 파일 + Makefile)
    - c5_embed 관련 파일 4개(embed_c5.go, embed_c5_stub.go, embed_c5_extract.go, embed_c5_test.go) + embed/hub/ 디렉토리는 삭제되어야 한다
    - hub_component.go (C5 서브프로세스 관리자)는 삭제되어야 한다
    - serve_components.go의 EmbeddedC5FS 참조는 제거되어야 한다
    - workerYAML 구조체의 Binary 필드 (C5 경로 참조)는 제거되어야 한다
    - worker.go (cq worker, C5 직접 실행 CLI)는 확인 후 제거되어야 한다
    - 변경 후 go build ./... && go vet ./... 통과해야 한다
  unwanted:
    - Edge 관련 코드(hub/edge.go, hub_edge_start.go) 제거
    - hub_worker.go의 Supabase 폴링 기능 제거
    - Hub MCP 도구 인터페이스 변경
    - c5_hub 태그로 보호되는 Hub 기능 자체 제거
out_of_scope:
    - Edge 기능 폐기
    - hub_worker.go 제거
    - MCP 도구명 변경