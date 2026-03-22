---
name: component-design
description: |
  React 컴포넌트 설계 가이드. props 인터페이스, 상태 관리, 합성 패턴, 재사용성을 체계적으로
  설계합니다. 새 컴포넌트를 만들거나 기존 컴포넌트를 리팩토링할 때 이 스킬을 사용하세요.
  "컴포넌트 설계", "component design", "React 컴포넌트", "props 설계", "상태 관리 패턴",
  "컴포넌트 분리", "공통 컴포넌트" 등의 요청에 트리거됩니다.
---

# Component Design

React 컴포넌트 설계 가이드.

## 트리거

"컴포넌트 설계", "component design", "React 컴포넌트", "props 설계", "상태 관리 패턴"

## Steps

### 1. 책임 정의

컴포넌트가 담당하는 것을 한 문장으로 정의:

- **표시 컴포넌트**: 데이터를 받아 렌더링만 (Button, Card, Avatar)
- **컨테이너 컴포넌트**: 데이터 페칭 + 상태 관리 (UserList, Dashboard)
- **레이아웃 컴포넌트**: 배치만 담당 (Sidebar, Grid, Stack)
- **페이지 컴포넌트**: 라우트 진입점 (HomePage, SettingsPage)

한 컴포넌트가 두 가지 이상 역할 → 분리 신호.

### 2. Props 인터페이스 설계

```typescript
// 좋은 예: 명확한 타입, 적절한 기본값
interface ButtonProps {
  variant: 'primary' | 'secondary' | 'danger';
  size?: 'sm' | 'md' | 'lg';       // 기본값: 'md'
  disabled?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}
```

**규칙:**
- Props 5개 이하 (초과 시 그룹화 또는 분리)
- boolean props: `isLoading`, `disabled` (긍정형 우선)
- 콜백: `on` prefix (`onClick`, `onChange`)
- 렌더 위임: `renderItem`, `children` 패턴
- `any` 금지, 유니온 타입 사용

### 3. 상태 관리 선택

| 상태 유형 | 도구 | 예시 |
|-----------|------|------|
| 로컬 UI | `useState` | 모달 열림, 입력값 |
| 서버 데이터 | TanStack Query | API 응답, 목록 |
| 복잡한 로컬 | `useReducer` | 폼, 멀티스텝 |
| 전역 클라이언트 | zustand | 테마, 사용자 설정 |
| URL 상태 | searchParams | 필터, 페이지네이션 |

**원칙**: 서버 상태와 클라이언트 상태를 섞지 않는다.

### 4. 합성 패턴

```typescript
// Compound Component — 관련 컴포넌트를 묶어서 사용
<Select>
  <Select.Trigger>선택하세요</Select.Trigger>
  <Select.Content>
    <Select.Item value="a">옵션 A</Select.Item>
    <Select.Item value="b">옵션 B</Select.Item>
  </Select.Content>
</Select>

// Render Props — 렌더링 위임
<DataFetcher url="/api/users">
  {({ data, loading }) => loading ? <Spinner /> : <UserList users={data} />}
</DataFetcher>

// Custom Hook — 로직 재사용
function useDebounce<T>(value: T, delay: number): T { ... }
```

### 5. 파일 구조

```
src/components/
  common/           ← 2개+ 페이지에서 사용
    Button/
      Button.tsx
      Button.test.tsx
      index.ts
  features/         ← 특정 기능 전용
    auth/
      LoginForm.tsx
      SignupForm.tsx
  layouts/          ← 레이아웃
    MainLayout.tsx
    Sidebar.tsx
```

### 6. 체크리스트

- [ ] 컴포넌트 책임이 한 문장으로 설명 가능한가?
- [ ] Props가 5개 이하인가?
- [ ] 200줄 이내인가? (초과 시 분리)
- [ ] 불필요한 re-render가 없는가? (`React.memo`, `useMemo`)
- [ ] 키보드 접근 가능한가? (`tabIndex`, `onKeyDown`)
- [ ] 에러 경계(Error Boundary)가 필요한가?

## 안티패턴

- Props drilling 3단계 이상 (Context 또는 zustand 사용)
- 컴포넌트 안에서 API 직접 호출 (커스텀 훅으로 분리)
- `useEffect` 남용 (파생 상태는 계산으로, 이벤트는 핸들러로)
- index.tsx에 모든 것 넣기 (파일 분리)
