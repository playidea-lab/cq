# C4 Plan Mode (Enhanced)

기획 문서를 해석하고, 구조화된 인터뷰를 통해 C4 태스크를 생성합니다.

## Instructions

### Phase 0: 상태 확인

1. `mcp__c4__c4_status` 호출하여 현재 상태 확인
2. INIT/HALTED 상태가 아니면:
   - PLAN: "이미 계획 모드입니다" 안내
   - EXECUTE/CHECKPOINT: 실행 중단 경고 후 확인 요청
3. INIT 상태면 `uv run c4 plan`으로 PLAN 모드 진입

---

### Phase 1: 기획 문서 스캔

프로젝트 루트와 `docs/` 폴더에서 기획 문서를 찾습니다.

**스캔 대상:**
- `*.md` (PRD, requirements, spec 등)
- `docs/*.md`
- `docs/**/*.md`

**실행:**
```
1. ls docs/ 또는 프로젝트 루트 확인
2. 기획 문서로 보이는 파일 식별:
   - PRD, prd, 기획, requirements, spec, plan 등 키워드 포함
   - 크기가 1KB 이상인 .md 파일
3. 발견된 문서 목록 출력
```

**출력 예시:**
```
📄 기획 문서 발견:
  - docs/PRD.md (11KB) - 제품 요구사항 문서
  - docs/API_SPEC.md (5KB) - API 스펙
```

---

### Phase 2: 문서 해석

각 기획 문서를 읽고 핵심 정보를 추출합니다.

**추출 항목:**
1. **프로젝트 개요**: 이름, 목표, 배경
2. **핵심 기능**: 주요 기능 목록
3. **기술 스택**: 언어, 프레임워크, 라이브러리
4. **개발 로드맵**: Phase/단계별 계획
5. **아키텍처**: 컴포넌트 구조, 데이터 흐름

**추출 시 주의:**
- `- [ ]` 체크리스트 → 잠재적 태스크
- `Phase N:` 또는 `단계 N:` → 체크포인트 후보
- 기술명 언급 → 기술 스택

**출력 예시:**
```
📋 문서 분석 결과:

프로젝트: AirSign - 허공 손끝 서명 인식 시스템

핵심 기능:
  1. 실시간 손끝 추적 (MediaPipe Hands)
  2. 펜업/펜다운 인식 (속도 기반)
  3. 필압 시뮬레이션
  4. 캘리그래피 렌더링
  5. 서명 검증 (DTW → Siamese)

기술 스택:
  - MediaPipe Hands
  - Canvas 2D
  - JavaScript

개발 로드맵:
  - Phase 1: 프로토타입 (8개 항목)
  - Phase 2: 검증 시스템 (5개 항목)
  - Phase 3: SDK 배포 (5개 항목)
  - Phase 4: 고도화 (4개 항목)

잠재적 태스크: 22개 식별됨
```

---

### Phase 3: 구조화된 인터뷰

문서에서 확인되지 않은 정보를 질문합니다.

#### 3.1 개발 환경 (문서에 없으면 질문)

```
❓ 개발 환경 확인:

1. 언어 선택:
   - [ ] Vanilla JavaScript
   - [ ] TypeScript (권장: 타입 안정성)

2. 빌드 도구:
   - [ ] 없음 (단순 프로젝트)
   - [ ] Vite (권장: 빠른 개발)
   - [ ] Webpack

3. 패키지 매니저:
   - [ ] npm
   - [ ] pnpm (권장)
```

#### 3.2 테스트 전략

```
❓ 테스트 전략:

1. 유닛 테스트:
   - [ ] 필요 없음
   - [ ] Vitest (권장)
   - [ ] Jest

2. E2E 테스트:
   - [ ] 필요 없음
   - [ ] Playwright
   - [ ] Cypress

3. 커버리지 목표: ___% (권장: 70%)
```

#### 3.3 품질 기준

```
❓ 코드 품질:

1. 린터:
   - [ ] ESLint (권장)
   - [ ] 없음

2. 포매터:
   - [ ] Prettier (권장)
   - [ ] 없음

3. 타입 검사 (JS인 경우):
   - [ ] JSDoc + TypeScript 검사
   - [ ] 없음
```

#### 3.4 C4 워크플로우

```
❓ C4 설정:

1. 체크포인트 위치:
   - [ ] Phase별 (권장: Phase 1 완료 후 리뷰)
   - [ ] 기능별 (주요 기능마다 리뷰)
   - [ ] 없음 (마지막에 한번)

2. 태스크 크기:
   - [ ] PRD 체크리스트 그대로 (권장)
   - [ ] 더 작게 분할
   - [ ] 더 크게 합침

3. 자동 실행 범위:
   - [ ] Phase 1만 (프로토타입까지)
   - [ ] Phase 1~2
   - [ ] 전체
```

---

### Phase 4: 태스크 생성

인터뷰 결과를 반영하여 C4 태스크를 생성합니다.

**생성 규칙:**
1. PRD의 체크리스트 항목 → 개별 태스크
2. `scope`는 영향받는 파일/디렉토리
3. `dod`는 구체적이고 검증 가능하게
4. `dependencies`는 선후관계 고려
5. `validations`는 인터뷰에서 결정된 도구

**태스크 생성:**
```javascript
// Phase 1 예시
mcp__c4__c4_add_todo({
  task_id: "T-001",
  title: "MediaPipe Hands 연동",
  scope: "src/HandTracker.js",
  dod: "웹캠에서 손 인식, 검지 손끝(landmark 8) 좌표 추출 가능"
})
```

**체크포인트 설정:**
`.c4/config.yaml`에 추가:
```yaml
checkpoints:
  - id: CP-001
    description: "Phase 1 프로토타입 완료"
    required_tasks: [T-001, T-002, ..., T-008]
    required_validations: [lint, unit]
```

---

### Phase 5: 계획 확정

생성된 계획을 요약하고 확인합니다.

**출력:**
```
✅ C4 계획 생성 완료

📊 요약:
  - 태스크: 22개 (Phase 1: 8개, Phase 2: 5개, ...)
  - 체크포인트: 4개
  - 예상 검증: lint, unit

📋 Phase 1 태스크:
  T-001: MediaPipe Hands 연동
  T-002: 손끝 좌표 추출
  T-003: 속도 기반 펜업/펜다운
  ...

🚀 다음 단계:
  /c4-run    - 실행 시작
  /c4-status - 상태 확인
```

**확인:**
- "진행" → `/c4-run` 안내
- "수정" → 어떤 부분 수정할지 질문
- "취소" → 태스크 삭제 후 재시작

---

## 빠른 참조

```
/c4-plan              # 전체 워크플로우 시작
/c4-plan --scan       # 문서 스캔만
/c4-plan --interview  # 인터뷰만 다시
```

## 관련 명령어

- `/c4-add-task` - 개별 태스크 추가
- `/c4-run` - 실행 시작
- `/c4-status` - 상태 확인
