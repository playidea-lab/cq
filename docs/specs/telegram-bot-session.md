feature: telegram-bot-session
domain: web-backend
summary: CQ 세션을 텔레그램 봇으로 대체하고, `cq` 기본 진입점을 claude+telegram으로 변경

requirements:
  ubiquitous:
    - "`cq` 실행 시 시스템은 프로젝트 봇 + 글로벌 봇 목록을 표시해야 한다"
    - "봇 선택 시 시스템은 해당 봇 토큰으로 claude+telegram 세션을 시작해야 한다"
    - "프로젝트 봇은 .c4/channels/telegram/bots/{username}/, 글로벌 봇은 ~/.claude/channels/telegram/bots/{username}/에 저장되어야 한다"

  event_driven:
    - "WHEN 사용자가 cq setup 실행 시 THEN 시스템은 BotFather 봇 생성 가이드를 step-by-step으로 표시한다"
    - "WHEN 사용자가 토큰을 입력하면 THEN 시스템은 Telegram API로 토큰 유효성을 검증하고, 봇 정보(username)를 저장한다"
    - "WHEN 토큰 검증 성공 시 THEN 시스템은 페어링 코드 입력을 대기한다"
    - "WHEN 페어링 완료 시 THEN 시스템은 즉시 claude+telegram 세션을 시작한다"
    - "WHEN 사용자가 새 봇 만들기를 선택하면 THEN 시스템은 cq setup 플로우로 진입한다"
    - "WHEN 사용자가 cq remove {bot} 실행 시 THEN 시스템은 로컬 봇 설정을 삭제한다"

  state_driven:
    - "WHILE 봇 목록이 비어있을 때 cq 실행 시 THEN 시스템은 cq setup 안내 메시지를 표시한다"
    - "WHILE 세션이 활성 상태일 때 THEN 봇 목록에서 활성/비활성/마지막 접속 상태를 표시한다"

  optional:
    - "IF 사용자가 cq claude 실행 시 THEN 시스템은 텔레그램 없이 로컬 터미널 세션을 시작한다"

  unwanted:
    - "cq {bot_name} 직접 지정으로 세션 시작 — 예약 명령어 충돌 위험"
    - "봇 생성 자동화 (BotFather API 없음)"
    - "-t 플래그로 세션 관리 (봇이 대체)"

non_functional:
  - "토큰 검증은 Telegram getMe API 호출 1회로 완료 (1초 이내)"
  - "봇 설정 파일은 JSON, 사람이 읽고 수동 편집 가능해야 함"
  - "-t 플래그는 deprecation 경고 후 봇 선택으로 리다이렉트"

out_of_scope:
  - "fly.io/서버 배포"
  - "Hub fallback LLM 연동"
  - "Discord/Slack 채널 어댑터"
  - "텔레그램 플러그인 자체 개발 (기존 공식 플러그인 활용)"

verification:
  type: cli
  commands:
    - "cq setup (위자드 플로우 검증)"
    - "cq (봇 목록 표시 검증)"
    - "cq list (봇 상태 표시 검증)"
    - "cq remove {bot} (삭제 검증)"
