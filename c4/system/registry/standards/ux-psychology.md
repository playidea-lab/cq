# UX Psychology Rules

> 70+ 심리학 원칙을 활용한 사용자 경험 설계

---

## 기본 사고 모델

### Occam's Razor (오컴의 면도날)

> "가장 단순한 설명이 보통 맞다."

```
적용: 복잡한 문제 전에 단순한 것부터 확인
- 폼이 안 보내지나? → 버튼 disabled 상태 확인
- 페이지 느리다? → 이미지 크기 확인
- 사용자가 이탈? → CTA 버튼 위치 확인
```

### Pareto Principle (80/20 법칙)

> "20%의 요소가 80%의 결과를 만든다."

```
적용: 핵심 요소에 집중
- 전환의 80%를 만드는 20% 페이지 최적화
- 사용자 80%가 쓰는 20% 기능에 집중
- 불필요한 기능 제거
```

### Theory of Constraints (제약 이론)

> "모든 시스템에는 하나의 병목이 있다."

```
진단 순서:
1. 트래픽 문제? → 마케팅/SEO
2. 전환 문제? → UX/카피
3. 리텐션 문제? → 제품/온보딩
```

---

## 구매 심리

### Mere Exposure Effect (단순 노출 효과)

> "반복 노출이 친숙함과 신뢰를 만든다."

```tsx
// ✅ GOOD: 일관된 디자인 패턴
<Button variant="primary" /> // 전체 사이트 동일
<Card radius="md" />         // 통일된 border-radius

// ❌ BAD: 페이지마다 다른 스타일
```

### Curse of Knowledge (지식의 저주)

> "전문가는 초보자 관점을 잊는다."

```
대응:
- 신규 사용자로 테스트
- 전문 용어 피하기
- "당연한" 단계도 설명
```

### Status-Quo Bias (현상 유지 편향)

> "사용자는 변화를 싫어한다."

```tsx
// ✅ GOOD: 변화 완화
<ImportWizard>
  "기존 데이터를 가져올까요?"
  <Button>Google에서 가져오기</Button>
  <Button>CSV 업로드</Button>
</ImportWizard>

// ❌ BAD: 빈 상태로 시작
```

### Peak-End Rule (정점-종점 법칙)

> "경험은 정점과 끝으로 기억된다."

```tsx
// ✅ GOOD: 기억에 남는 순간 설계
// Peak: 첫 성공 순간에 축하 애니메이션
<Confetti trigger={firstSuccess} />

// End: 완료 페이지에 인상적인 요약
<CompletionPage>
  <SuccessAnimation />
  <AchievementBadge />
  <ShareButton />
</CompletionPage>
```

### Zeigarnik Effect (자이가르닉 효과)

> "미완성 작업은 기억에 남는다."

```tsx
// ✅ GOOD: 진행률 표시로 완료 유도
<ProgressBar value={80} label="프로필 80% 완성" />

<OnboardingChecklist>
  <Item done>이메일 인증 ✓</Item>
  <Item done>프로필 사진 ✓</Item>
  <Item pending>팀 초대 (마지막 단계!)</Item>
</OnboardingChecklist>
```

---

## 설득 원칙

### Reciprocity (상호성)

> "받으면 갚고 싶어진다."

```tsx
// ✅ GOOD: 먼저 가치 제공
<FreeTool />           // 무료 계산기
<FreeTemplate />       // 무료 템플릿
<FreeTrial days={14} /> // 무료 체험

// 그 다음 요청
<UpgradePrompt />
```

### Commitment & Consistency (일관성)

> "작은 약속이 큰 행동으로 이어진다."

```tsx
// ✅ GOOD: 단계별 진행
<Step1>이메일만 입력</Step1>      // 낮은 장벽
<Step2>이름 추가</Step2>          // 이미 시작함
<Step3>결제 정보</Step3>          // 포기하기 아까움

// ❌ BAD: 한 번에 모든 정보 요구
<BigForm fields={15} />
```

### Loss Aversion (손실 회피)

> "얻는 것보다 잃는 것이 2배 아프다."

```tsx
// ✅ GOOD: 손실 프레이밍
<Alert type="warning">
  "무료 체험이 3일 남았습니다. 업그레이드하지 않으면 데이터가 삭제됩니다."
</Alert>

<Feature locked>
  "Pro에서만 사용 가능 - 이 기능 없이 계속하시겠습니까?"
</Feature>

// ❌ BAD: 이득만 강조
"Pro로 업그레이드하면 더 많은 기능!"
```

### Scarcity (희소성)

> "희귀할수록 가치있어 보인다."

```tsx
// ✅ GOOD: 진정한 희소성 표시
<Badge>남은 자리: 3/50</Badge>
<Timer>특가 종료까지: 2:34:12</Timer>

// ⚠️ 주의: 가짜 희소성은 신뢰 파괴
```

### Authority Bias (권위 편향)

> "전문가 의견을 더 신뢰한다."

```tsx
// ✅ GOOD: 권위 표시
<TrustBar>
  <Logo client="Google" />
  <Logo client="Microsoft" />
  <Certification name="SOC 2" />
  <Quote author="CTO, Fortune 500" />
</TrustBar>
```

### Social Proof (사회적 증거)

> "다른 사람들이 하면 따라한다."

```tsx
// ✅ GOOD: 다양한 사회적 증거
<Stats>
  <Stat value="50,000+" label="활성 사용자" />
  <Stat value="4.8/5" label="평균 평점" />
</Stats>

<Testimonials />
<RecentActivity>"John이 방금 가입했습니다"</RecentActivity>
<TrendingBadge>이번 주 인기</TrendingBadge>
```

---

## 가격/선택 설계

### Hick's Law (힉의 법칙)

> "선택지가 많을수록 결정이 느려진다."

```tsx
// ✅ GOOD: 3개 이하 옵션
<PricingTable>
  <Plan name="Basic" />
  <Plan name="Pro" recommended />  // 추천 표시
  <Plan name="Enterprise" />
</PricingTable>

// ❌ BAD: 너무 많은 옵션
<PricingTable plans={7} />
```

### Paradox of Choice (선택의 역설)

> "너무 많은 선택은 마비를 유발한다."

```tsx
// ✅ GOOD: 기본값 제공
<Select defaultValue="recommended">
  <Option value="recommended">추천 설정</Option>
  <Option value="custom">사용자 정의</Option>
</Select>
```

### Decoy Effect (미끼 효과)

> "비교 대상이 선택에 영향을 준다."

```tsx
// ✅ GOOD: 전략적 가격 배치
<Plan name="Basic" price={9} features={5} />
<Plan name="Pro" price={19} features={15} recommended />  // 목표
<Plan name="Plus" price={15} features={8} />  // 미끼: Pro가 더 좋아 보임
```

### Charm Pricing (매력 가격)

```tsx
// 가치 제품: .99 사용
<Price>$9.99</Price>

// 프리미엄 제품: 라운드 넘버
<Price>$500</Price>
```

---

## 마찰 & 흐름

### Activation Energy (활성화 에너지)

> "첫 단계가 어려우면 시작하지 않는다."

```tsx
// ✅ GOOD: 마찰 최소화
<Form>
  <Input placeholder="이메일" autoFocus />
  <Button>시작하기</Button>  // 이메일만으로 시작
</Form>

// ❌ BAD: 높은 진입 장벽
<Form fields={["name", "email", "phone", "company", "size", "industry"]} />
```

### BJ Fogg Behavior Model

> "행동 = 동기 × 능력 × 촉발"

```tsx
// 동기: 왜 해야 하는지
<ValueProposition>"30% 시간 절약"</ValueProposition>

// 능력: 쉽게 할 수 있는지
<OneClickSignup provider="Google" />

// 촉발: 지금 하라는 신호
<CTA prominent>무료로 시작하기</CTA>
```

### Goal-Gradient Effect (목표 기울기 효과)

> "목표에 가까울수록 동기가 높아진다."

```tsx
// ✅ GOOD: 진행 상황 시각화
<CheckoutProgress>
  <Step done>장바구니 ✓</Step>
  <Step done>배송 정보 ✓</Step>
  <Step current>결제 (마지막!)</Step>
</CheckoutProgress>

// 스타벅스 앱: "2잔 더 마시면 무료 음료!"
```

---

## 리텐션 & 참여

### Endowment Effect (소유 효과)

> "가진 것에 더 높은 가치를 부여한다."

```tsx
// ✅ GOOD: 소유감 생성
<FreeTrial>
  14일 동안 "당신의" 워크스페이스
  - 데이터 저장됨
  - 설정 커스터마이징
  - 팀원 초대 가능
</FreeTrial>
```

### IKEA Effect (이케아 효과)

> "직접 만들면 더 가치있게 느낀다."

```tsx
// ✅ GOOD: 사용자 참여 유도
<Onboarding>
  <Step>테마 색상 선택</Step>
  <Step>대시보드 커스터마이징</Step>
  <Step>첫 프로젝트 생성</Step>
</Onboarding>

// 직접 만들었으니 떠나기 아까움
```

### Switching Costs (전환 비용)

> "떠나기 어렵게 만든다 (윤리적으로)."

```tsx
// ✅ GOOD: 가치 축적
- 데이터 축적 (히스토리, 분석)
- 커스터마이징 (설정, 워크플로우)
- 팀 채택 (협업, 권한)
- 통합 연결 (API, 연동)

// ⚠️ 주의: 데이터 내보내기는 항상 제공
```

---

## UX 문제별 적용 가이드

| 문제 | 적용할 원칙 |
|------|------------|
| **높은 이탈률** | Activation Energy, Hick's Law, BJ Fogg |
| **폼 미완료** | Goal-Gradient, Zeigarnik, Commitment |
| **가격 혼란** | Paradox of Choice, Hick's Law, Decoy |
| **낮은 신뢰** | Social Proof, Authority, Reciprocity |
| **기능 과다** | Occam's Razor, Pareto, Hick's Law |
| **리텐션 문제** | Endowment, IKEA Effect, Switching Costs |
| **온보딩 이탈** | Activation Energy, Peak-End, Progress |

---

## 체크리스트

### 설계 전
- [ ] 핵심 문제 정의 (트래픽? 전환? 리텐션?)
- [ ] 대상 사용자 심리 파악
- [ ] 적용할 원칙 선택

### 설계 중
- [ ] 마찰 최소화 (Activation Energy)
- [ ] 진행률 표시 (Goal-Gradient)
- [ ] 사회적 증거 배치 (Social Proof)
- [ ] 명확한 CTA (BJ Fogg)

### 설계 후
- [ ] A/B 테스트로 검증
- [ ] 원칙 적용 효과 측정
- [ ] 반복 개선

---

## 참고

- [Marketing Psychology Skill](https://github.com/coreyhaines31/marketingskills)
- [Influence: The Psychology of Persuasion](https://www.amazon.com/Influence-Psychology-Persuasion-Robert-Cialdini/dp/006124189X)
- [Hooked: How to Build Habit-Forming Products](https://www.amazon.com/Hooked-How-Build-Habit-Forming-Products/dp/1591847788)
