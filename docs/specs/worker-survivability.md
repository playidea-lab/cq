feature: worker-survivability
domain: infra
summary: "cq serve install 한 번으로 OS 재부팅/크래시 후 자동 복구되는 워커"

requirements:
  ubiquitous:
    - "cq serve install 실행 시, 시스템은 OS에 적합한 서비스를 자동 등록한다"
    - "등록된 서비스는 OS 부팅 시 자동 시작된다"
    - "서비스가 크래시하면 자동 재시작된다"

  event_driven:
    - "WHEN OS가 macOS THEN LaunchAgent (KeepAlive + RunAtLoad) 등록 — 이미 구현됨"
    - "WHEN OS가 Linux (non-WSL) THEN systemd user unit (Restart=always, WantedBy=default.target) 등록"
    - "WHEN OS가 WSL2 THEN systemd user unit + Windows Task Scheduler 이중 등록"
    - "WHEN nvidia-smi가 PATH에 없고 /usr/lib/wsl/lib/nvidia-smi 존재 THEN 해당 경로로 GPU 감지"
    - "WHEN relay 서버가 워커 offline 감지 THEN Telegram 알림 발송"

  state_driven:
    - "WHILE 서비스 실행 중 THEN relay 연결 유지 및 토큰 자동 갱신"

  unwanted:
    - "시스템은 sudo 권한을 요구하지 않는다 (user-level 서비스)"
    - "외부 의존성(Tailscale, autossh 등)을 요구하지 않는다"
    - "cq serve uninstall로 깨끗하게 제거 가능해야 한다"

non_functional:
  - "WSL2 감지: /proc/version에 microsoft 포함 여부"
  - "서비스 재시작 딜레이: 5초 이내"
  - "토큰 갱신: TokenProvider 5분 margin + .mcp.json 45분 주기 동기화"

out_of_scope:
  - "Windows native (non-WSL) 서비스 등록"
  - "Docker 컨테이너 환경"
  - "자동 cq 바이너리 업데이트"
