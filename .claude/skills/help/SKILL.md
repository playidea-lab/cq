---
name: help
description: |
  Quick reference for C4 skills, agents, and tools. Provides summaries,
  decision trees, and keyword search across 22 skills, 37 agents (9 categories),
  and 133 MCP tools (107 base + 26 hub). Use when the user needs help, reference,
  skill list, or wants to search C4 capabilities. Triggers: "도움말",
  "명령어 목록", "도구 검색", "help", "list commands", "show agents",
  "what tools", "how to".
allowed-tools: Read, Glob, Grep, mcp__cq__*
context: fork
---

# C4 Help

Quick reference for commands, agents, and tools. Parse `$ARGUMENTS` and branch accordingly.

## Usage

```
/help              → Full summary
/help commands     → All skills
/help agents       → Agents by category
/help tools        → MCP tools (3 layers)
/help <keyword>    → Keyword search
```

## Instructions

Parse `$ARGUMENTS` for branch. No MCP calls needed (static output).

**Feature matrix**: 기능별 의사결정이 필요할 때 (drive vs transfer 등) `references/feature-matrix.md` 참조.

### No args → Decision Tree + Core Commands

```
What's the task?
├─ 1-line fix → Just fix it (no C4)
├─ Small (1-5 files) → /quick "desc" → /submit
├─ Medium (5-15 files) → /add-task → /run  OR  c4_claim → c4_report
├─ Large (15+ files) → /plan → /run N  OR  /swarm N
├─ Research/experiment → /c9-loop, /review (paper)
├─ Document work → c4_parse_document, /proposal-writer, /document-review
└─ Idea exploration → /pi

Core: /status, /quick, /run, /submit, /validate
More: /help commands | agents | tools | piki | <keyword>
```

### "piki" → Piki 표준 스킬 안내

```
🔹 CQ 스킬 (c9- prefix 등): CQ MCP 도구 의존. CQ 프로젝트에서만 동작.
   /plan, /run, /finish, /review (논문), /c9-loop 등 30개

🔸 piki 스킬 (prefix 없음): 범용 워크플로우 가이드. CQ 없이도 활용 가능.
   cq standards apply로 설치. 24개:

   [auto-install — 모든 프로젝트]
   /company-review    코드 리뷰 (6축, soul.md 기반)
   /pr-review         PR 체크리스트 + 리뷰 가이드
   /incident-response 장애 대응 플로우
   /claude-md-improver CLAUDE.md 분석/개선

   [문서 작성]
   /proposal-writer   제안서/입찰서 작성
   /document-review   사내 문서 리뷰
   /meeting-notes     회의록 작성
   /doc-writing       ADR/스펙/README
   /internal-comms    사내 공지/리포트

   [개발 워크플로우]
   /feature-dev       7단계 기능 개발
   /refactor          안전한 리팩토링
   /migration         DB/API 마이그레이션
   /deploy            프로덕션 배포

   [품질/보안]
   /security-audit    보안 감사 체크리스트
   /perf-audit        성능 감사
   /webapp-testing    E2E 테스트 (Playwright)

   [기획/관리]
   /estimation        작업량 추정
   /postmortem        포스트모템 작성

   [도구/자동화]
   /mcp-builder       MCP 서버 개발
   /hookify           커스텀 훅 생성
   /automation-recommender 자동화 추천
   /onboarding        프로젝트 온보딩
   /data-pipeline     ETL 파이프라인
```

### "commands" → See `references/commands.md`

### "agents" → See `references/agents.md`

### "tools" → See `references/tools.md`

### Other → Keyword search across `references/search-data.md`

Output matching commands, agents, and tools. If no matches: "No results. Use /help for full list."
