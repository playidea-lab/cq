---
name: incident-runbook
description: |
  장애 대응 런북. 감지→심각도 판단→초기 조치→복구→포스트모템 순서로 진행.
  트리거: "장애", "인시던트", "incident", "서비스 다운", "에러율 급증"
allowed-tools: Read, Bash
---
# Incident Runbook

서비스 장애를 체계적으로 대응합니다.

## Phase 1: 감지 & 심각도 판단 (0-5분)

### 심각도 분류

| 레벨 | 기준 | 대응 시간 |
|------|------|----------|
| P1 — Critical | 전체 서비스 다운, 데이터 손실 | 즉시 |
| P2 — High | 주요 기능 장애, 50% 이상 사용자 영향 | 15분 내 |
| P3 — Medium | 일부 기능 장애, 우회 가능 | 1시간 내 |
| P4 — Low | 경미한 이슈, 사용자 영향 없음 | 다음 영업일 |

### 초기 상태 확인

```bash
# 서비스 상태
curl -I https://<service-url>/health

# 최근 에러 로그
journalctl -u <service> --since "10 minutes ago" | grep ERROR

# 리소스 확인
top -bn1 | head -20
df -h
free -h
```

## Phase 2: 초기 조치 (5-15분)

- [ ] 인시던트 채널 생성 (#incident-YYYYMMDD-<요약>)
- [ ] IC(Incident Commander) 지정
- [ ] 상태 페이지 업데이트 (사용자 공지)
- [ ] 관련 팀 알림

### 빠른 진단

```bash
# 최근 배포 확인
git log --oneline -10
# 또는
<deploy-tool> history --limit 5

# DB 연결 상태
<db-tool> status

# 외부 의존성 확인
curl -I https://<external-service>/ping
```

## Phase 3: 근본 원인 분석 (15-30분)

원인 후보:
- [ ] 최근 배포 (코드 변경)
- [ ] 인프라 변경 (설정, 리소스)
- [ ] 트래픽 급증
- [ ] 외부 의존성 장애
- [ ] 데이터 문제

```bash
# 에러 패턴 분석
grep -E "ERROR|FATAL|PANIC" <log-file> | sort | uniq -c | sort -rn | head -20

# 타임라인 구성
grep -E "ERROR" <log-file> | awk '{print $1, $2}' | uniq -c
```

## Phase 4: 조치 & 복구

### 롤백 (빠른 복구)

```bash
# 직전 버전으로 롤백
<deploy-tool> rollback

# 확인
curl -I https://<service-url>/health
```

### 픽스 배포 (원인 확인 후)

- [ ] 코드 수정 및 리뷰
- [ ] Staging 검증
- [ ] 프로덕션 배포
- [ ] 모니터링 5분

## Phase 5: 복구 확인

- [ ] 헬스체크 정상
- [ ] 에러율 기준 이하로 복귀
- [ ] 주요 기능 동작 확인
- [ ] 상태 페이지 "resolved" 업데이트
- [ ] 사용자 공지

## Phase 6: 포스트모템

인시던트 종료 후 24시간 내 작성:

```markdown
## 포스트모템: <인시던트 요약>

**발생**: YYYY-MM-DD HH:MM
**복구**: YYYY-MM-DD HH:MM
**영향 시간**: X분

### 타임라인
- HH:MM — 감지
- HH:MM — 원인 특정
- HH:MM — 조치 시작
- HH:MM — 복구 완료

### 근본 원인

### 재발 방지 액션 아이템
- [ ] 액션1 (담당자, 기한)
- [ ] 액션2 (담당자, 기한)
```

# CUSTOMIZE: 알림 채널, 상태 페이지 URL, 배포 도구 이름 설정
# 예: 슬랙 채널 #incidents, PagerDuty escalation policy
