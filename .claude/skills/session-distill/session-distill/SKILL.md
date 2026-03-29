---
name: session-distill
essential: true
description: >
  Distill conversation context into CQ knowledge records at session end or on demand.
  Extracts decisions, insights, strategy, and discoveries from human-agent dialogue
  and saves them via c4_knowledge_record. Use when: (1) user says "정리해줘",
  "지식 저장", "distill", "save context", "세션 정리"; (2) at natural session
  boundaries after significant discussion; (3) user says "이 대화 기억해",
  "이거 저장", "remember this". Requires CQ MCP connection (c4_knowledge_record tool).
---

# Session Distill

Scan the current conversation, extract knowledge worth preserving, and save each
piece via `c4_knowledge_record`. One conversation may yield 0–5 knowledge entries.

## Workflow

### 1. Scan conversation for extractable knowledge

Review the full conversation history. Identify entries that match these categories:

| Category | doc_type | Example |
|----------|----------|---------|
| Product/business decision | `insight` | "CQ의 핵심 가치는 누적 지식 레이어" |
| Technical discovery | `pattern` | "acceptEdits 모드는 자동수락이지 무음허용이 아님" |
| Architecture/design choice | `insight` | "MCP 프로토콜이라 어떤 클라이언트든 연결 가능" |
| Competitor/market analysis | `insight` | "Devin은 SaaS, CQ는 로컬 CLI + 분산 워커" |
| Bug root cause | `pattern` | "permissions.allow에 있으면 PermissionRequest 미발생" |
| User preference/feedback | `insight` | "고객 불편 Top3: 권한 프롬프트, 설정 복잡도, 도구 과잉" |
| Research hypothesis | `hypothesis` | "세션 종료 hook으로 자동 증류 가능할 것" |

**Skip**: ephemeral task details, things already in code/git, simple Q&A with no novel insight.

### 2. Draft knowledge entries

For each extracted item, prepare:

```
title:   Concise 1-line summary (Korean or English, match conversation language)
content: 2-5 sentences of context + reasoning + conclusion (markdown)
doc_type: insight | pattern | hypothesis
tags:    [relevant, tags]
domain:  product | architecture | devops | ml | ux | business (pick closest)
scope:   project (default) | global (if broadly applicable)
```

### 3. Deduplicate against existing knowledge

Before recording, call `c4_knowledge_search(query=<title>)` for each entry.
If a highly similar result exists (same core information), skip or update instead.

### 4. Record via MCP

Call `c4_knowledge_record` for each entry. Example:

```
c4_knowledge_record(
  doc_type: "insight",
  title: "CQ 핵심 가치: 에이전트가 일할수록 프로젝트가 똑똑해지는 누적 지식 레이어",
  content: "Devin/Cursor는 세션이 끝나면 컨텍스트가 사라지지만, CQ는 knowledge_record, experiment_record, persona_learn이 DB에 남아 다음 세션이 이전보다 나아지는 구조. MCP 프로토콜이라 Claude Code, Codex, Cursor 등 어떤 클라이언트든 같은 SSOT에 접근 가능.",
  tags: ["product-strategy", "positioning", "knowledge-system"],
  domain: "product",
  scope: "project"
)
```

### 5. Report summary

After recording, output a brief summary:

```
## Session Distill Complete

Saved N knowledge entries:
- [insight] "제목" (domain: X)
- [pattern] "제목" (domain: Y)

Skipped M items (already recorded or ephemeral).
```

## Guidelines

- **Quality over quantity**: 1 good insight > 5 trivial notes
- **Preserve the "why"**: Record reasoning and context, not just conclusions
- **Tag consistently**: Reuse existing tags from `c4_knowledge_search` results
- **Language**: Match the conversation language (Korean if Korean, English if English)
- **No secrets**: Never record API keys, passwords, or PII
- **Confidence**: Set 0.7+ for confirmed decisions, 0.3-0.6 for hypotheses
