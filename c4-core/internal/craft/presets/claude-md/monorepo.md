---
name: monorepo
description: 모노레포 프로젝트용 CLAUDE.md — 패키지 구조, 빌드 시스템, 공유 설정 컨벤션
---

# Project Instructions

## Build & Test

```bash
# 전체 빌드
pnpm build        # 또는 nx build, turbo build

# 특정 패키지
pnpm --filter @<scope>/<package> build
pnpm --filter @<scope>/<package> test

# 영향받는 패키지만
pnpm nx affected --target=test
```

## Package Structure

```
packages/
  shared/          — 공통 타입, 유틸리티
  ui/              — 공유 UI 컴포넌트
  config/          — 공유 설정 (tsconfig, eslint, etc.)
apps/
  web/             — 웹 애플리케이션
  api/             — 백엔드 API
  mobile/          — 모바일 앱
```

## 의존성 규칙

- 앱 → 패키지 의존 허용
- 패키지 → 앱 의존 금지
- 패키지 간 순환 의존 금지
- 공유 타입은 `packages/shared`에 정의

## Code Style

- 각 패키지는 독립적으로 빌드/테스트 가능
- 패키지 간 import는 패키지 이름으로: `import { Foo } from '@scope/shared'`
- 상대 경로로 다른 패키지 import 금지 (`../../packages/...`)
- 각 패키지의 `package.json` `exports` 필드로 공개 API 명시

## Versioning

- 패키지 버전 동기화: workspace protocol 사용 (`"@scope/shared": "workspace:*"`)
- 배포 가능한 패키지: Semantic Versioning
- 내부 전용 패키지: 버전 불필요 (`"private": true`)

## CI/CD

- 변경된 패키지만 빌드/테스트/배포 (affected builds)
- 공유 패키지 변경 시 모든 의존 패키지 재빌드

# CUSTOMIZE: 빌드 도구 (Turborepo/Nx/Lerna), 패키지 매니저 (pnpm/yarn workspaces), 스코프 이름
