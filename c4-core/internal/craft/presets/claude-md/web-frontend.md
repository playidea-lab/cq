---
name: web-frontend
description: Web 프론트엔드용 CLAUDE.md — React/TypeScript 빌드와 프론트엔드 컨벤션
---

# Project Instructions

## Build & Test

```bash
pnpm install
pnpm build
pnpm test
pnpm lint
```

## Code Style

- TypeScript strict mode
- Functional components with hooks
- No `any` type — use proper generics
- CSS Modules or Tailwind (no inline styles)

## Component Pattern

- One component per file
- Props interface exported
- Default exports for pages, named exports for components

# CUSTOMIZE: 라우팅, 상태 관리, API 연동 패턴 추가

## Project Structure

<!-- 프로젝트 디렉토리 구조를 여기에 -->
