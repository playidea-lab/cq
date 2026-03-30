feature: ML Experiment Management
domain: go-backend

requirements:
  # 프로젝트 초기화
  - type: event-driven
    id: REQ-INIT-01
    text: "WHEN 사용자가 cq init 실행 시 THEN 시스템은 git repo 존재를 확인하고, 없으면 git init을 실행한다"
  - type: event-driven
    id: REQ-INIT-02
    text: "WHEN git repo가 확인되면 THEN 시스템은 .cqdata 파일이 없으면 빈 템플릿을 생성하고 git에 추가한다"

  # 코드 버전 (git 강제)
  - type: event-driven
    id: REQ-GIT-01
    text: "WHEN 워커가 실험 태스크를 받을 때 IF git working tree가 dirty이면 THEN 시스템은 경고를 출력하고 자동 커밋을 제안한다"
  - type: event-driven
    id: REQ-GIT-02
    text: "WHEN 실험이 시작되면 THEN 시스템은 현재 git commit SHA를 experiment_record에 기록한다"

  # 데이터 버전 (Drive Dataset + .cqdata)
  - type: event-driven
    id: REQ-DATA-01
    text: "WHEN 워커가 dataset 필드가 포함된 태스크를 받을 때 THEN 시스템은 .cqdata에서 해당 dataset의 name+version을 읽어 drive dataset pull을 자동 실행한다"
  - type: event-driven
    id: REQ-DATA-02
    text: "WHEN drive dataset upload 완료 시 THEN 시스템은 .cqdata의 해당 dataset 항목을 업데이트하고 git add를 제안한다"
  - type: event-driven
    id: REQ-DATA-03
    text: "WHEN .cqdata에 없는 dataset을 참조할 때 THEN 시스템은 최신 버전을 pull하고 .cqdata에 추가를 제안한다"

  # Config 버전
  - type: event-driven
    id: REQ-CFG-01
    text: "WHEN 실험이 시작되면 THEN 시스템은 사용된 config 파일의 git 상태를 확인하고, untracked이면 git add를 제안한다"
  - type: event-driven
    id: REQ-CFG-02
    text: "WHEN experiment_record에 기록할 때 THEN config 내용의 hash를 함께 저장한다"

non_functional:
  - ".cqdata 파싱: 1ms 이내 (단순 YAML)"
  - "Drive Dataset pull: 기존 성능 유지 (병렬 4 goroutine)"
  - "git 연산: exec.Command 사용, gitgo 의존성 추가 안 함"

out_of_scope:
  - "DVC 통합 (Drive Dataset으로 대체)"
  - "실험 비교 UI (CLI 출력만)"
  - "아티팩트 자동 수집 (에이전트 판단에 위임)"
  - "Hydra config 관리 (YAML + git으로 시작)"
