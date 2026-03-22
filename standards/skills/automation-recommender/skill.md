# Automation Recommender

코드베이스를 분석하여 자동화 기회를 추천.

## 트리거

"자동화 추천", "automation recommender", "뭘 자동화할까", "개선 포인트", "DX 개선"

## Steps

### 1. 프로젝트 스캔

현재 프로젝트의 자동화 상태를 점검:

```bash
# 기존 설정 확인
ls .claude/hooks/ 2>/dev/null          # 훅
ls .claude/skills/ 2>/dev/null         # 스킬
cat .claude/settings.json 2>/dev/null  # 설정
cat .mcp.json 2>/dev/null              # MCP 서버
cat .github/workflows/ 2>/dev/null     # CI/CD
```

### 2. 자동화 카테고리

| 카테고리 | 도구 | 예시 |
|----------|------|------|
| **코드 품질** | 훅, CI | 커밋 전 린트, PR 시 자동 리뷰 |
| **보안** | 훅, CI | 시크릿 스캔, 위험 명령 차단 |
| **워크플로우** | 스킬, 커맨드 | `/deploy`, `/release`, `/review` |
| **컨텍스트** | MCP 서버 | DB 조회, API 호출, 문서 검색 |
| **모니터링** | CI, 훅 | 의존성 업데이트, 성능 회귀 |

### 3. 추천 로직

프로젝트 상태에 따라 추천:

**없으면 추천:**
- `hooks/` 없음 → bash-security.sh + stop-guard.sh
- 린트 설정 없음 → 언어별 린트 CI
- `.gitignore`에 시크릿 패턴 없음 → 추가
- 테스트 CI 없음 → 테스트 자동화

**있으면 확장 추천:**
- CI 있음 → 자동 리뷰, 성능 벤치마크 추가
- 훅 있음 → 커스텀 규칙 (hookify) 추가
- MCP 있음 → 프로젝트 맞춤 도구 추가

### 4. 출력 형식

```markdown
## 자동화 추천 보고서

### 현재 상태
- 훅: N개 설치
- 스킬: N개 활성
- CI: [있음/없음]
- MCP: [있음/없음]

### 추천 (우선순위순)

#### 1. [높음] 보안 훅 추가
- **현재**: 위험 명령 차단 없음
- **추천**: bash-security.sh 설치
- **적용**: `cq standards apply`

#### 2. [중간] PR 자동 리뷰
- **현재**: 수동 리뷰만
- **추천**: code-review 스킬 + CI 연동
- **적용**: `.github/workflows/review.yml` 추가

#### 3. [낮음] 커스텀 MCP 도구
- **현재**: 기본 도구만
- **추천**: DB 조회 도구 추가
- **적용**: MCP 서버 개발
```

### 5. 적용 가이드

각 추천에 구체적 적용 방법 포함:
- 명령어 또는 파일 경로
- 예상 효과
- 소요 시간 추정

## 안티패턴

- 모든 것을 자동화 (ROI 고려)
- 복잡한 자동화를 먼저 (간단한 것부터)
- 자동화 후 모니터링 안 함
