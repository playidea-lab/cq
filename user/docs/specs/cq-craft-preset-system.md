feature: CQ Craft — 프리셋 기반 커스텀 도구 시스템
domain: cli
stakeholders: [개발자 (CQ 사용자)]

requirements:
  ubiquitous:
    - U1: 생성된 도구는 ~/.claude/{skills|agents|rules}/에 표준 포맷으로 저장된다
    - U2: agent/skill/rule 구분은 CQ가 자동 판단한다 (사용자에게 묻지 않음)
    - U3: 프리셋은 80% 완성 상태 + CUSTOMIZE 마커로 수정 포인트 표시

  event_driven:
    - E1: WHEN `cq add <preset>` THEN 프리셋 카탈로그에서 ~/.claude/에 복사 후 에디터 열기
    - E2: WHEN `cq craft` THEN 대화형 인터뷰 시작 → 프리셋 매칭 or 새로 생성
    - E3: WHEN 세션 종료 시 반복 패턴 3회 이상 감지 THEN "스킬로 만들까요?" 제안
    - E4: WHEN `cq list --mine` THEN 내 커스텀 도구 목록 표시 (프로젝트 내장과 구분)
    - E5: WHEN `cq remove <name>` THEN 해당 도구 파일 삭제

  optional:
    - O1: IF 프리셋에 비슷한 것이 있으면 THEN craft 시 "이거에서 시작할까?" 제안
    - O2: IF `cq share <name>` THEN 공유 가능 포맷으로 내보내기 (향후)

  unwanted:
    - W1: 사용자에게 "이건 skill인가요 agent인가요?" 질문 금지
    - W2: 실시간 제안으로 작업 흐름 방해 금지 (세션 끝에만)

non_functional:
  - 프리셋 설치는 1초 이내
  - craft 대화는 5턴 이내에 결과물 생성
  - 기존 cq 바이너리에 포함 (별도 설치 불필요)

out_of_scope:
  - 커뮤니티 마켓플레이스 (향후)
  - 프리셋 자동 업데이트
  - 팀 공유 기능
