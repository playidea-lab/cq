---
name: a11y-audit
description: |
  웹 접근성(Accessibility) 감사 가이드. WCAG 2.1 AA 기준으로 키보드 네비게이션, 스크린리더,
  색상 대비, 시맨틱 HTML 등을 점검합니다. 접근성 개선이 필요하거나 출시 전 a11y 점검 시
  반드시 이 스킬을 사용하세요. "접근성", "a11y", "accessibility", "WCAG", "스크린리더",
  "키보드 네비게이션", "색상 대비" 등의 요청에 트리거됩니다.
---

# A11y Audit

웹 접근성 감사 가이드 (WCAG 2.1 AA).

## 트리거

"접근성", "a11y", "accessibility", "WCAG", "스크린리더", "키보드 네비게이션"

## Steps

### 1. 시맨틱 HTML 점검

- [ ] `<button>` vs `<div onClick>` — 네이티브 요소 우선
- [ ] 제목 계층 순서: `h1` → `h2` → `h3` (건너뛰기 금지)
- [ ] 랜드마크: `<nav>`, `<main>`, `<aside>`, `<footer>` 사용
- [ ] 목록: `<ul>/<ol>/<li>` (관련 항목 그룹화)
- [ ] 폼: `<label>` + `for` 속성, `<fieldset>` + `<legend>`

### 2. 키보드 네비게이션

- [ ] 모든 인터랙티브 요소가 Tab으로 접근 가능
- [ ] 포커스 순서가 시각적 순서와 일치
- [ ] 포커스 표시(outline)가 보이는가? (`outline: none` 금지)
- [ ] 모달: 포커스 트랩 (모달 밖으로 Tab 이동 방지)
- [ ] ESC로 모달/드롭다운 닫기
- [ ] Enter/Space로 버튼 활성화
- [ ] 화살표 키로 메뉴/탭/라디오 이동

### 3. 스크린리더 호환

- [ ] 이미지: `alt` 텍스트 (장식용은 `alt=""`)
- [ ] 아이콘 버튼: `aria-label` 또는 숨겨진 텍스트
- [ ] 동적 콘텐츠: `aria-live="polite"` (알림, 로딩 상태)
- [ ] 토글: `aria-expanded`, `aria-pressed`
- [ ] 탭: `role="tablist"`, `aria-selected`
- [ ] 에러 메시지: `aria-describedby`로 입력 필드와 연결

### 4. 색상/대비

- [ ] 텍스트 대비: 4.5:1 이상 (일반), 3:1 이상 (큰 텍스트)
- [ ] 색상만으로 정보 전달 금지 (아이콘, 패턴, 텍스트 병용)
- [ ] 다크 모드에서도 대비 기준 충족
- [ ] 포커스 인디케이터: 배경과 3:1 이상 대비

### 5. 반응형/모바일

- [ ] 터치 타겟: 44x44px 이상
- [ ] 핀치 줌 차단 금지 (`user-scalable=no` 금지)
- [ ] 가로 스크롤 없이 320px에서 사용 가능
- [ ] 화면 회전 제한 금지

### 6. 테스트 도구

```bash
# axe-core (자동 검사)
pnpm add -D @axe-core/react
# 또는 브라우저 확장: axe DevTools

# Lighthouse 접근성 점수
lighthouse --only-categories=accessibility https://localhost:3000

# eslint-plugin-jsx-a11y (정적 분석)
pnpm add -D eslint-plugin-jsx-a11y
```

### 7. 보고서 형식

```markdown
## A11y 감사 보고서
- **일시**: YYYY-MM-DD
- **기준**: WCAG 2.1 AA
- **도구**: axe-core, Lighthouse, 수동 테스트
- **점수**: Lighthouse a11y N/100

### 발견 사항
| 심각도 | 위치 | 문제 | WCAG 기준 | 수정 방법 |
|--------|------|------|-----------|----------|
| Critical | LoginForm | label 없음 | 1.3.1 | `<label>` 추가 |
```

## 안티패턴

- `div`에 `onClick`만 달고 키보드 이벤트 없음
- `aria-label`을 시맨틱 HTML 대신 남용
- "스크린리더 사용자는 없을 거야" 가정
- `outline: none` 전역 설정 (포커스 표시 제거)
