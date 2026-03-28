---
name: deploy-checklist
description: |
  배포 전 확인 항목 체크리스트. 빌드, 테스트, 환경변수, 롤백 계획, DB 마이그레이션 포함.
  트리거: "배포해줘", "deploy", "배포 전 체크", "deploy-checklist", "릴리즈 준비"
allowed-tools: Read, Bash, Glob
---
# Deploy Checklist

배포 전 필수 확인 항목을 순서대로 체크합니다.

## 실행 순서

### Step 1: 현재 상태 확인

```bash
git status
git log --oneline -5
git branch --show-current
```

### Step 2: 빌드 & 테스트 검증

# CUSTOMIZE: 프로젝트 빌드/테스트 명령으로 교체하세요
```bash
# Go 프로젝트
go build ./... && go vet ./...
go test ./... -count=1

# Node.js 프로젝트
npm run build
npm test

# Python 프로젝트
uv run python -m py_compile src/
uv run pytest
```

### Step 3: 체크리스트 출력

아래 항목을 하나씩 확인하고 결과를 보고한다.

#### 코드 품질
- [ ] 빌드 성공
- [ ] 테스트 전체 통과
- [ ] lint/vet 경고 없음
- [ ] 민감 정보(시크릿, API키) 코드에 없음

#### 환경 설정
- [ ] 환경변수 프로덕션 값으로 설정 확인
- [ ] Feature flag 상태 확인
- [ ] 외부 서비스 의존성(DB, 캐시, 메시지큐) 접근 가능

#### DB / 데이터
- [ ] 마이그레이션 스크립트 검토 완료
- [ ] 마이그레이션 롤백 스크립트 준비
- [ ] 데이터 백업 완료 (파괴적 변경 시)

#### 롤백 계획
- [ ] 롤백 절차 문서화
- [ ] 이전 버전 이미지/바이너리 접근 가능
- [ ] 롤백 소요 시간 추정: ___분

#### 모니터링
- [ ] 배포 후 확인할 메트릭/로그 대시보드 준비
- [ ] 알림 임계값 적절히 설정
- [ ] 헬스체크 엔드포인트 확인

### Step 4: 최종 Go/No-Go 판정

모든 항목 통과 시:
```
배포 준비 완료. Go!
배포 시각: YYYY-MM-DD HH:MM
담당자: <이름>
```

미통과 항목이 있으면 No-Go 사유와 조치 계획 명시.
