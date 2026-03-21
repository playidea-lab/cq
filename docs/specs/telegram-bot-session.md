feature: telegram-bot-session
domain: web-backend
summary: CQ CLI에서 -t(세션)와 --bot(텔레그램)을 독립적으로 조합하여 사용
status: implemented (2026-03-22)

requirements:
  ubiquitous:
    - "`cq` 실행 시 순수 claude가 즉시 시작된다 (텔레그램 없음)"
    - "`cq --bot` 실행 시 봇 선택 메뉴를 표시한다"
    - "`cq --bot <name>` 실행 시 해당 봇으로 텔레그램을 연결한다"
    - "`cq -t <name>` 실행 시 --session-id로 UUID를 고정하여 named session을 시작/재개한다"
    - "-t와 --bot은 독립적으로 조합 가능하다"

  event_driven:
    - "WHEN --bot 메뉴에서 새 봇 만들기 선택 시 THEN 인라인 setup wizard가 실행된다"
    - "WHEN 봇 토큰 입력 시 THEN Telegram getMe API로 검증 후 botstore에 저장한다"
    - "WHEN --bot 없이 실행 시 THEN CQ_NO_TELEGRAM=1 설정으로 텔레그램 플러그인을 차단한다"
    - "WHEN 봇 선택 시 THEN C4_TELEGRAM_BOT_TOKEN 환경변수로 토큰을 주입한다"

  state_driven:
    - "WHILE named session UUID가 존재할 때 THEN --resume UUID로 세션을 재개한다"
    - "WHILE named session이 없을 때 THEN --session-id UUID로 새 세션을 생성하고 UUID를 저장한다"

  optional:
    - "IF cq --bot 메뉴에서 봇이 없을 때 THEN 새 봇 만들기 / 취소 선택지를 표시한다"

  unwanted:
    - "cq setup 별도 명령 (--bot → 새 봇 만들기로 통합)"
    - "cq 기본 실행 시 봇 메뉴 강제 표시"

non_functional:
  - "토큰 검증은 Telegram getMe API 호출 1회로 완료 (1초 이내)"
  - "봇 설정은 botstore JSON으로 관리"
  - "세션 UUID는 ~/.c4/named-sessions.json에 저장"
  - "멀티봇: 환경변수가 .env 파일을 오버라이드하여 봇 간 충돌 없음"

out_of_scope:
  - "fly.io/서버 배포"
  - "Hub fallback LLM 연동"
  - "Discord/Slack 채널 어댑터"
  - "텔레그램 플러그인 자체 개발 (기존 공식 플러그인 활용)"

verification:
  type: cli
  commands:
    - "cq (순수 claude 실행 검증)"
    - "cq -t mywork (named session 생성/재개 검증)"
    - "cq --bot (봇 메뉴 표시 검증)"
    - "cq --bot cqbot (봇 직접 지정 검증)"
    - "cq -t mywork --bot cqbot (조합 검증)"
    - "cq ls (봇 목록 검증)"
    - "cq sessions (세션 목록 검증)"
