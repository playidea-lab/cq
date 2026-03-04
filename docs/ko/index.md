---
layout: home

hero:
  name: "CQ"
  text: "AI 프로젝트 자동화 엔진"
  tagline: 계획 → 구현 → 리뷰 → 배포. Claude Code로 자동화.
  actions:
    - theme: brand
      text: 시작하기
      link: /ko/guide/install
    - theme: alt
      text: 빠른 시작
      link: /ko/guide/quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: 🗂️
    title: 구조화된 워크플로우
    details: 모든 태스크에는 완료 조건(DoD), 자동 리뷰, 체크포인트 게이트가 있습니다. "완료"에 대한 모호함이 없습니다.

  - icon: ⚡
    title: 병렬 워커
    details: 격리된 워크트리에서 여러 Claude Code 에이전트를 병렬 실행합니다. 각 워커는 한 태스크에 집중합니다.

  - icon: 🧠
    title: 지식 누적
    details: 결정 사항과 발견이 자동으로 기록됩니다. 미래 태스크는 과거 패턴에서 학습합니다.

  - icon: 🔒
    title: 기본 보안
    details: AES-256-GCM 시크릿 스토어. API 키는 절대 설정 파일에 저장되지 않습니다. pre-commit 훅으로 셸 명령을 검토합니다.

  - icon: 🏷️
    title: 이름 붙은 세션
    details: "`cq claude -t <name>`으로 언제든지 세션을 재개합니다. `cq ls`로 세션 목록과 미읽은 메일을 확인합니다."

  - icon: 📬
    title: 세션 간 메일
    details: CLI 또는 MCP 도구로 세션 간 메시지를 주고받습니다. 터미널에서 벗어나지 않고 병렬 에이전트를 조율합니다.

  - icon: 🩺
    title: 자동 복구
    details: StaleChecker가 멈춘 in_progress 태스크를 자동으로 감지하고 초기화합니다. 더 이상 수동으로 해제할 필요가 없습니다.

  - icon: ☁️
    title: 원커맨드 클라우드 설정
    details: "`cq auth login`으로 GitHub OAuth를 열고 Supabase 자격증명을 자동 설정합니다. 수동 설정 편집 불필요."

  - icon: 🛡️
    title: 워크플로우 게이트
    details: 훅 기반 품질 강제. `/c4-finish` 완료 전에는 `git commit`이 차단됩니다. 더 이상 필요 없는 스킬(`/c4-polish`, `/c4-refine`)은 자동으로 리다이렉트됩니다.

  - icon: 📱
    title: 헤드리스 인증
    details: "`cq auth login --device`로 브라우저에 입력할 user_code를 표시합니다 (RFC 8628 Device Flow). SSH 터널과 컨테이너에서도 동작합니다."
---
