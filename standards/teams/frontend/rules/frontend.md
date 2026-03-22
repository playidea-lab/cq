# Frontend Team Rules

> React + TypeScript 프론트엔드 개발 규칙.
> 적용: `cq init --team frontend`

---

## 컴포넌트 설계

- 컴포넌트 크기: 200줄 초과 시 분리 검토.
- Props: 5개 초과 시 객체로 묶거나 컴포넌트 분리 검토.
- 비즈니스 로직: 컴포넌트에서 분리 → custom hook 또는 utils.
- 재사용 컴포넌트: `src/components/common/`에 위치.

## 상태 관리

- 서버 상태: TanStack Query (React Query). 직접 fetch + useState 금지.
- 클라이언트 상태: zustand (복잡한 경우), useState (단순한 경우).
- form 상태: react-hook-form + zod 검증.
- URL 상태: 필터, 정렬, 페이지네이션은 URL search params로.

## 성능

- 리스트 렌더링: `React.memo` + 안정적 key.
- 큰 리스트: 가상화 (tanstack-virtual).
- 이미지: lazy loading + 적절한 포맷 (WebP 우선).
- 번들: code splitting — 라우트 단위 lazy import.

## 스타일

- CSS: Tailwind CSS 기본. CSS-in-JS 지양.
- 반응형: mobile-first. breakpoint는 Tailwind 기본값 사용.
- 디자인 토큰: 색상/간격/폰트는 변수로 관리 (하드코딩 금지).
- 다크 모드: CSS 변수 기반. 시스템 설정 우선.

## 접근성

- 시맨틱 HTML: `<button>`, `<nav>`, `<main>` 등 적절한 태그.
- 키보드 네비게이션: 모든 인터랙티브 요소 접근 가능.
- aria 속성: 커스텀 위젯에 필수.
- 색상 대비: WCAG AA 기준 충족.

## API 통신

- API 클라이언트: 중앙 인스턴스 (`src/lib/api.ts`).
- 에러 처리: global error boundary + toast 알림.
- 로딩 상태: skeleton UI. 스피너는 최후 수단.
- 낙관적 업데이트: mutation 성공 확률 높은 경우 적용.

## 테스트

- 컴포넌트: Testing Library (`@testing-library/react`).
- 인터랙션 테스트: `userEvent` 사용 (`fireEvent` 지양).
- 스냅샷 테스트: 지양 — 깨지기 쉽고 리뷰 어려움.
- E2E: Playwright. 핵심 사용자 플로우만.

## CQ 연동 (CQ 프로젝트인 경우)

| 작업 | CQ 도구/스킬 |
|------|-------------|
| 컴포넌트/페이지 설계 | `/c4-plan` |
| 구현 실행 | `/c4-run` |
| 린트·테스트 검증 | `/c4-validate` |
| 구현 마무리 (빌드, 커밋) | `/c4-finish` |
| UI 패턴/이슈 이력 조회 | `c4_knowledge_search` |
