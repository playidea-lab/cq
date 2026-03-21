# TypeScript Style Guide

> TypeScript 프로젝트 코딩 컨벤션.

## 도구

- 패키지 관리: `pnpm` (npm, yarn 대신)
- 린트: `pnpm lint` (ESLint)
- 포매터: Prettier (ESLint 연동)
- 빌드: `pnpm build`
- 테스트: `pnpm test` (Vitest 또는 Jest)

## TypeScript 설정

- `strict: true` 필수 (tsconfig.json).
- `noImplicitAny: true` — 암묵적 any 금지.
- `strictNullChecks: true` — null/undefined 명시적 처리.

## 패턴

- **타입 추론 활용**: 명백한 경우 타입 생략 가능. 함수 반환은 명시 권장.
- **interface vs type**: 확장 가능한 객체는 `interface`, 유니온/유틸리티는 `type`.
- **enum 지양**: `const object + as const` 또는 union type 선호.
- **Optional chaining**: `?.` 사용, 중첩 if 지양.
- **Nullish coalescing**: `??` 사용 (`||` 대신 — falsy 값 구분).

## React (해당 시)

- 함수 컴포넌트 + hooks. class 컴포넌트 금지.
- 상태 관리: 로컬은 useState, 복잡한 경우 useReducer 또는 zustand.
- key prop: index 사용 지양 — 안정적인 고유 값 사용.
- useEffect 의존성 배열: 빠짐없이 명시.

## 프로젝트 구조

```
src/
  components/    UI 컴포넌트
  hooks/         커스텀 훅
  utils/         유틸리티
  types/         타입 정의
  styles/        CSS/스타일
```

## 금지

- `any` 타입 — 불가피하면 `unknown` + 타입 가드.
- `@ts-ignore` — `@ts-expect-error` + 사유 주석 사용.
- `var` — `const` 기본, 재할당 필요 시 `let`.
- 비동기 에러 무시 (`.catch()` 없는 Promise).
