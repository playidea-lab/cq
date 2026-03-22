# Webapp Testing

E2E 웹 애플리케이션 테스트 가이드 (Playwright).

## 트리거

"E2E 테스트", "webapp testing", "Playwright", "브라우저 테스트", "UI 테스트", "통합 테스트"

## Steps

### 1. 테스트 범위 결정

E2E 테스트는 비용이 높다 — 핵심 플로우만:

- 로그인/인증 플로우
- 핵심 비즈니스 플로우 (주문, 결제, 등록 등)
- 치명적 에러 시나리오
- 외부 연동 플로우 (OAuth, 결제 게이트웨이)

단위 테스트로 커버 가능한 것은 E2E로 테스트하지 않는다.

### 2. 테스트 구조

```
tests/
├── e2e/
│   ├── auth.spec.ts         # 인증 플로우
│   ├── checkout.spec.ts     # 핵심 비즈니스
│   └── fixtures/
│       ├── test-data.ts     # 테스트 데이터
│       └── pages/           # Page Object Model
│           ├── login.page.ts
│           └── dashboard.page.ts
```

### 3. Page Object Model

페이지별 셀렉터와 액션을 캡슐화:

```typescript
// pages/login.page.ts
export class LoginPage {
  constructor(private page: Page) {}

  async login(email: string, password: string) {
    await this.page.fill('[data-testid="email"]', email);
    await this.page.fill('[data-testid="password"]', password);
    await this.page.click('[data-testid="submit"]');
  }

  async expectError(message: string) {
    await expect(this.page.locator('[data-testid="error"]')).toHaveText(message);
  }
}
```

### 4. 셀렉터 우선순위

1. `data-testid` (가장 안정적, 리팩토링에 강함)
2. ARIA role (`getByRole('button', { name: 'Submit' })`)
3. Text content (`getByText('로그인')`)
4. CSS selector (최후 수단, 깨지기 쉬움)

`#id`, `.class`, `tag` 셀렉터 사용 금지 — 스타일 변경에 깨짐.

### 5. 테스트 작성 원칙

- 각 테스트는 독립적 (공유 상태 금지)
- 테스트 데이터는 테스트 시작 시 생성, 종료 시 정리
- 네트워크 대기: `waitForResponse` / `waitForSelector` (sleep 금지)
- 재시도 로직 내장: flaky 테스트는 3회 재시도 후 실패 판정
- 스크린샷: 실패 시 자동 캡처 (디버깅용)

### 6. CI 연동

```yaml
# 예시: GitHub Actions
e2e:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - run: pnpm install
    - run: pnpm exec playwright install --with-deps
    - run: pnpm exec playwright test
    - uses: actions/upload-artifact@v4
      if: failure()
      with:
        name: playwright-report
        path: playwright-report/
```

### 7. 모니터링

- CI에서 실행 시간 추적 (느려지면 병렬화)
- Flaky 테스트 비율 추적 (5% 이상이면 정리 필요)
- 실패 스크린샷 + 트레이스 자동 수집

## 안티패턴

- 모든 기능을 E2E로 테스트 (느리고 비쌈)
- `sleep(3000)` 으로 대기
- 프로덕션 데이터에 의존하는 테스트
- CSS class 셀렉터로 UI 요소 찾기
