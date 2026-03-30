feature: cq-standards-cmd
domain: go-backend
summary: cq standards 서브커맨드 — 표준 관리를 cq claude에서 분리

requirements:
  ubiquitous:
    - "cq standards는 .piki-lock.yaml 기반으로 현재 적용된 표준 상태를 출력해야 한다"
    - "cq standards apply는 team.lang 형식의 인자를 파싱해야 한다"
    - "cq standards check는 현재 파일과 표준의 차이를 출력해야 한다"
    - "cq standards list는 사용 가능한 team/lang 목록을 출력해야 한다"
    - "Apply()는 팀 변경 시 이전 lock에 있지만 새 lock에 없는 파일을 삭제해야 한다"

  event_driven:
    - "WHEN cq standards apply backend.go 실행 THEN team=backend, langs=[go]로 파싱하여 Apply 호출"
    - "WHEN apply 시 수정된 파일 발견 THEN --force 없으면 skip + 경고"
    - "WHEN 팀 변경 시 이전 규칙 존재 THEN 이전 lock 기반으로 불필요 파일 제거"

  unwanted:
    - "cq claude에 --team/--lang 플래그가 남아있으면 안 된다"
    - "수정된 파일을 --force 없이 덮어쓰면 안 된다"
    - "lock 파일 없는 상태에서 cq standards가 에러를 내면 안 된다 (미적용 안내)"

out_of_scope:
  - "원격 piki 리포에서 pull (v2)"
  - "자동 업데이트 알림"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/standards/..."
