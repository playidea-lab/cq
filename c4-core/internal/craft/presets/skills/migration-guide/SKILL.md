---
name: migration-guide
description: |
  DB 마이그레이션 안전 실행 체크리스트. 백업→테스트→실행→검증→롤백 계획 순서로 진행.
  트리거: "마이그레이션", "migration", "DB 변경", "스키마 변경"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---
# Migration Guide

데이터베이스 마이그레이션을 안전하게 실행합니다.

## 실행 순서

### Step 1: 마이그레이션 파일 검토

```bash
# 마이그레이션 파일 목록 확인
ls -la migrations/
# 또는
ls -la db/migrations/
```

최신 마이그레이션 파일을 Read로 읽어 내용 파악:
- DDL 변경 (CREATE/ALTER/DROP TABLE)
- 데이터 변경 (UPDATE/INSERT/DELETE)
- 인덱스 변경

### Step 2: 영향 범위 분석

- [ ] 어떤 테이블이 변경되는가?
- [ ] 변경이 기존 쿼리와 호환되는가? (backward compatible)
- [ ] 롤백 가능한가? (`DOWN` 마이그레이션 존재 확인)
- [ ] 대용량 테이블 변경 시 락 발생 가능성 확인
- [ ] 실행 예상 시간 (EXPLAIN ANALYZE)

### Step 3: 사전 백업

```bash
# PostgreSQL
pg_dump -h $DB_HOST -U $DB_USER -d $DB_NAME > backup_$(date +%Y%m%d_%H%M%S).sql

# MySQL
mysqldump -h $DB_HOST -u $DB_USER -p $DB_NAME > backup_$(date +%Y%m%d_%H%M%S).sql
```

- [ ] 백업 파일 크기 확인
- [ ] 백업 복원 테스트 (staging 환경)

### Step 4: Staging 환경 테스트

```bash
# staging DB에서 먼저 실행
ENV=staging <migration-tool> migrate up

# 결과 확인
<migration-tool> status
```

- [ ] 마이그레이션 성공 확인
- [ ] 애플리케이션 정상 동작 확인
- [ ] 롤백 테스트

### Step 5: 프로덕션 실행

```bash
# 유지보수 모드 (필요 시)
# 마이그레이션 실행
<migration-tool> migrate up

# 실행 시간 기록
time <migration-tool> migrate up
```

### Step 6: 실행 후 검증

- [ ] `<migration-tool> status` — 모든 마이그레이션 applied
- [ ] 주요 쿼리 실행 시간 확인
- [ ] 에러 로그 확인 (최소 5분 모니터링)
- [ ] 애플리케이션 헬스체크

### Step 7: 롤백 계획

문제 발생 시:

```bash
# 직전 버전으로 롤백
<migration-tool> migrate down 1

# 또는 백업 복원
psql -h $DB_HOST -U $DB_USER -d $DB_NAME < backup_YYYYMMDD_HHMMSS.sql
```

## 체크리스트 요약

```
## 마이그레이션 체크리스트

사전:
- [ ] 마이그레이션 파일 리뷰 완료
- [ ] 롤백 스크립트 존재 확인
- [ ] 백업 완료
- [ ] Staging 테스트 완료

실행:
- [ ] 저트래픽 시간대 선택
- [ ] 팀 공지
- [ ] 모니터링 대기 중

사후:
- [ ] 마이그레이션 상태 확인
- [ ] 애플리케이션 정상 확인
- [ ] 5분 모니터링 완료
```

# CUSTOMIZE: 마이그레이션 도구 지정
# 예: golang-migrate, Flyway, Liquibase, Alembic, Prisma Migrate
# 예: 팀 특화 백업 스크립트 경로
