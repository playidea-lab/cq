# C4 문서 작성 로드맵 (Document Checklist)

본 문서는 **c4 / c4d 시스템 구현을 위해 이후 작성되어야 할 문서 목록**을 정리한 체크리스트입니다.
각 문서는 구현·운영·확장을 단계적으로 가능하게 하기 위한 목적을 가집니다.

---

## A. 핵심 필수 문서 (v0 필수)

### 1. C4 PRD (Product Requirements Document)
- 목적: C4 시스템의 목표/비목표를 명확히 정의
- 대상 독자: 제품 설계자, 핵심 개발자
- 주요 내용:
  - 문제 정의
  - 해결 목표
  - 핵심 사용자(Leader / Worker / Supervisor)
  - 성공 지표(KPI)

### 2. C4 사용자 여정 & 운영 플로우
- 목적: 실제 사용 흐름을 명확히 고정
- 주요 내용:
  - `/c4 init → plan → run → checkpoint → complete`
  - 멀티 터미널 / 멀티 워커 시나리오
  - 워커 join / submit / supervisor approve 흐름

### 3. C4 상태머신 명세
- 목적: 시스템 상태 전이의 단일 기준 정의
- 주요 내용:
  - PLAN / EXECUTE / CHECKPOINT / COMPLETE / REPAIR
  - 이벤트 기반 상태 전이 규칙
  - 불변 조건(invariant)

### 4. todo.md 포맷 & Task 스키마 명세
- 목적: 사람이 쓰는 todo를 시스템이 해석 가능하게 표준화
- 주요 내용:
  - task 필드(id, scope, priority, deps, DoD, validations)
  - REQUEST_CHANGES → todo 자동 변환 규칙
  - 예제 todo.md

### 5. Checkpoint / Gate 명세
- 목적: 품질과 범위를 통제하는 기준 고정
- 주요 내용:
  - CP0 / CP1 / CP2 정의
  - Gate 체크리스트 템플릿
  - 자동 승인 vs Supervisor 승인 기준

### 6. Supervisor Headless 프롬프트 표준
- 파일: `prompt_supervisor.md`
- 목적: Supervisor 판단을 자동화 가능한 함수 형태로 고정
- 주요 내용:
  - 입력(bundle 구성)
  - 출력 JSON 스키마
  - 허용 도구 정책

### 7. Worker 프롬프트 표준 (Ralph Loop)
- 목적: 워커의 행동을 반복 가능하게 제한
- 주요 내용:
  - 작업 단위 규칙
  - 실패 시 행동 프로토콜
  - 테스트 → 수정 → 재시도 루프

### 8. C4 CLI 명세서
- 목적: 사용자 인터페이스 표준화
- 주요 내용:
  - `/c4`, `/c4d` 명령 목록
  - 인자, 출력, exit code
  - 에러 규약

### 9. C4d IPC / API 명세서
- 목적: 데몬과 클라이언트 간 통신 표준화
- 주요 내용:
  - API 엔드포인트
  - 요청/응답 JSON 스키마
  - 인증 방식

---

## B. 구현 안정성을 위한 설계 문서 (v1 권장)

### 10. 이벤트 카탈로그 & Replay 규칙
- 이벤트 타입별 의미/스키마
- 멱등성, 중복 처리 규칙

### 11. 락 & 동시성 정책 문서
- leader lock
- scope lock
- TTL / stale lock 복구 전략

### 12. Git 연동 정책
- 브랜치/커밋 규칙
- merge 정책
- 충돌 처리 전략

### 13. 보안 / 안전 가이드
- 허용 명령 allowlist
- 위험 동작 차단 정책
- 민감 정보 로그 방지

---

## C. 배포 · 운영 문서 (v1 ~ v2)

### 14. 설치 가이드
- Claude Code 설정
- c4 / c4d 설치 및 실행

### 15. 운영자 Runbook
- 데몬 장애 대응
- 무한 루프 / 실패 폭주 대응
- repair mode 진입 절차

### 16. 관측 / 로그 / 리포팅 문서
- 로그 구조
- 이벤트 조회 방법
- status 출력 예시

---

## D. 품질 · 확장 문서 (추천)

### 17. 테스트 전략 문서
- 단위 / 통합 / 시뮬레이션 테스트
- 가짜 repo 기반 시나리오

### 18. 아키텍처 문서
- 모듈 구조(c4d / cli / plugin)
- 데이터 흐름(state / events / bundles)
- 확장 포인트(MCP, 원격 워커)

### 19. ADR 템플릿 & ADR 목록
- 중요한 설계 결정 기록
- 변경 이력 추적

---

## 요약

- **A 영역 문서가 완성되면 c4 v0 구현 가능**
- **B 영역은 멀티 워커 안정화**
- **C/D 영역은 장기 운영 및 플랫폼화 단계**

이 문서는 C4 개발의 "지도" 역할을 합니다.
