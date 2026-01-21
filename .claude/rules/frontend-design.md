# Frontend Design Rules

> 제네릭 AI 스타일을 피하고, 독창적이고 기억에 남는 UI를 만든다.

---

## 디자인 사고 (코딩 전 필수)

코드 작성 전에 **명확한 미적 방향**을 결정한다:

### 1. 목적 파악
- 이 인터페이스가 해결하는 문제는?
- 사용자는 누구인가?

### 2. 톤 선택 (하나를 명확히)

| 스타일 | 특징 |
|--------|------|
| **Brutally Minimal** | 극도의 절제, 여백, 타이포 중심 |
| **Maximalist** | 풍부한 요소, 레이어, 밀도 |
| **Retro-Futuristic** | 과거와 미래의 융합, 네온, 그리드 |
| **Organic/Natural** | 부드러운 곡선, 자연 색상, 흐름 |
| **Luxury/Refined** | 절제된 우아함, 고급 타이포 |
| **Playful** | 밝은 색상, 재미있는 인터랙션 |
| **Editorial** | 매거진 스타일, 대담한 타이포 |
| **Brutalist** | 원시적, 거친 테두리, 시스템 폰트 |
| **Art Deco** | 기하학적, 금색 악센트, 대칭 |
| **Industrial** | 기능적, 어두운 톤, 유틸리티 |

### 3. 차별화 포인트
- 사용자가 **기억할 한 가지**는 무엇인가?
- 경쟁 제품과 **다른 점**은?

---

## 타이포그래피

### CRITICAL - 제네릭 폰트 금지

```css
/* ❌ BAD: 과도하게 사용되는 폰트 */
font-family: Arial, Inter, Roboto, system-ui;

/* ✅ GOOD: 개성 있는 폰트 선택 */
/* Display: 제목용 - 독특하고 기억에 남는 */
font-family: 'Playfair Display', 'Space Grotesk', 'Clash Display';

/* Body: 본문용 - 가독성 + 개성 */
font-family: 'Source Serif Pro', 'IBM Plex Sans', 'Outfit';
```

### HIGH - 폰트 페어링

| 용도 | 추천 조합 |
|------|----------|
| **Luxury** | Playfair Display + Source Serif Pro |
| **Modern** | Space Grotesk + Inter (본문만) |
| **Editorial** | Clash Display + IBM Plex Serif |
| **Playful** | Fredoka + Nunito |
| **Technical** | JetBrains Mono + IBM Plex Sans |

---

## 컬러 & 테마

### CSS 변수 필수

```css
/* ✅ GOOD: 일관된 테마 시스템 */
:root {
  --color-primary: #2563eb;
  --color-accent: #f59e0b;
  --color-background: #fafafa;
  --color-text: #1f2937;
  --color-muted: #6b7280;

  --spacing-unit: 8px;
  --radius-sm: 4px;
  --radius-md: 8px;
  --radius-lg: 16px;
}
```

### HIGH - 대담한 팔레트

```css
/* ❌ BAD: 소심한 균등 분배 */
background: #f5f5f5;
color: #666;
border: 1px solid #ddd;

/* ✅ GOOD: 지배색 + 강한 악센트 */
background: #0a0a0a;
color: #fafafa;
accent-color: #22d3ee;  /* 시안 악센트 */
```

---

## 모션 & 애니메이션

### CSS 우선, 라이브러리 보조

```css
/* ✅ GOOD: CSS 기반 마이크로인터랙션 */
.button {
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.button:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

/* ✅ GOOD: 스태거드 애니메이션 */
.list-item {
  opacity: 0;
  animation: fadeInUp 0.4s ease forwards;
}

.list-item:nth-child(1) { animation-delay: 0.1s; }
.list-item:nth-child(2) { animation-delay: 0.2s; }
.list-item:nth-child(3) { animation-delay: 0.3s; }

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
```

### React: Framer Motion 활용

```tsx
// ✅ GOOD: 의미 있는 전환
<motion.div
  initial={{ opacity: 0, y: 20 }}
  animate={{ opacity: 1, y: 0 }}
  exit={{ opacity: 0, y: -20 }}
  transition={{ duration: 0.3, ease: "easeOut" }}
>
  {content}
</motion.div>
```

---

## 공간 구성

### 예상을 깨는 레이아웃

| 기법 | 설명 |
|------|------|
| **비대칭** | 의도적 불균형으로 시선 유도 |
| **오버랩** | 요소 겹침으로 깊이감 |
| **그리드 이탈** | 일부 요소만 그리드 벗어남 |
| **과감한 여백** | 빈 공간도 디자인 요소 |
| **대각선 흐름** | 시선을 사선으로 유도 |

```css
/* ✅ GOOD: 비대칭 + 오버랩 */
.hero {
  display: grid;
  grid-template-columns: 1fr 1.5fr;
  gap: 0; /* 의도적 겹침 */
}

.hero-image {
  margin-left: -60px; /* 오버랩 */
  z-index: 1;
}

.hero-text {
  padding: 80px 40px;
  margin-top: 120px; /* 비대칭 */
}
```

---

## 배경 & 시각 효과

### 단색 배경 지양

```css
/* ❌ BAD: 밋밋한 단색 */
background: #ffffff;

/* ✅ GOOD: 깊이감 있는 배경 */

/* 그라디언트 메쉬 */
background:
  radial-gradient(at 20% 80%, #818cf8 0%, transparent 50%),
  radial-gradient(at 80% 20%, #22d3ee 0%, transparent 50%),
  #0f172a;

/* 노이즈 텍스처 */
background-image: url("data:image/svg+xml,...noise...");
background-size: 200px;
opacity: 0.03;

/* 기하학 패턴 */
background-image:
  linear-gradient(30deg, #f0f0f0 12%, transparent 12.5%),
  linear-gradient(150deg, #f0f0f0 12%, transparent 12.5%);
background-size: 20px 35px;
```

---

## 안티패턴 (CRITICAL - 금지)

### 제네릭 AI 스타일

| 패턴 | 문제점 |
|------|--------|
| **Inter/Roboto/Arial** | 모든 AI가 선택하는 폰트 |
| **보라 그라디언트 + 흰 배경** | AI 생성물의 클리셰 |
| **둥근 카드 + 그림자** | 2020년대 초반 스타일 |
| **예측 가능한 레이아웃** | 헤더-히어로-3컬럼-푸터 |
| **무난한 색상** | #666 텍스트, #f5f5f5 배경 |

```css
/* ❌ BAD: 전형적인 AI 생성 스타일 */
.card {
  background: white;
  border-radius: 12px;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
  padding: 24px;
}

/* ✅ GOOD: 컨텍스트에 맞는 독창적 스타일 */
.card {
  background: linear-gradient(135deg, #1e1b4b 0%, #312e81 100%);
  border: 1px solid rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(10px);
  padding: 32px 24px;
}
```

---

## 체크리스트

### 코딩 전
- [ ] 미적 방향 결정 (10가지 중 선택)
- [ ] 차별화 포인트 정의
- [ ] 폰트 페어링 선택

### 코딩 중
- [ ] CSS 변수로 테마 관리
- [ ] 제네릭 폰트 사용 안 함
- [ ] 의미 있는 애니메이션 적용
- [ ] 레이아웃에 변화 줌

### 코딩 후
- [ ] "이게 AI가 만든 것처럼 보이나?" → 보이면 수정
- [ ] "사용자가 기억할 한 가지가 있나?" → 없으면 추가

---

## 참고 자료

- [Refactoring UI](https://www.refactoringui.com/)
- [Awwwards](https://www.awwwards.com/)
- [Google Fonts](https://fonts.google.com/)
- [Framer Motion](https://www.framer.com/motion/)
