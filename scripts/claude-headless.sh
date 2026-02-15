#!/bin/bash
# claude-headless.sh — C4 프로젝트용 headless Claude Code 패턴 모음
#
# Usage: ./scripts/claude-headless.sh <command> [args...]
#
# Commands:
#   fix-test <package>     — 실패 테스트 자동 수정
#   verify                 — 전체 빌드 + 테스트 검증
#   review <file>          — 단일 파일 코드 리뷰
#   migration-check        — SQL 마이그레이션 스키마 검증
#   refactor <plan-file>   — 계획 기반 다중 파일 리팩토링 (자기 검증)
#   explore <topic>        — 병렬 탐색 → 구현 계획 생성

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

ALLOWED_RO="Read,Grep,Glob"
ALLOWED_RW="Read,Edit,Bash,Grep,Glob"

case "${1:-help}" in

  fix-test)
    PKG="${2:?Usage: fix-test <package-path>}"
    unset CLAUDECODE
    claude -p "
Go 패키지 ${PKG}에서 실패하는 테스트를 수정하라.

1. \`go test -v ${PKG}\`를 실행하여 실패 테스트 확인
2. 각 실패에 대해:
   a) 테스트 코드를 읽고 기대 동작 파악
   b) 소스 코드에서 근본 원인 추적
   c) 최소한의 수정 적용
   d) 해당 테스트만 재실행하여 통과 확인
3. 전체 패키지 테스트 재실행
4. 결과 요약 출력 (수정 파일, 변경 내용)

수정하지 못한 테스트가 있으면 이유와 함께 보고.
테스트가 실제로 통과할 때만 '완료'로 보고.
" \
      --allowedTools "$ALLOWED_RW" \
      --model sonnet \
      --print
    ;;

  verify)
    unset CLAUDECODE
    claude -p "
C4 프로젝트 전체 빌드 + 테스트 검증:

1. \`cd c4-core && go build ./...\` — 빌드 오류 확인
2. \`cd c4-core && go vet ./...\` — 정적 분석
3. \`cd c4-core && go test -p 1 ./...\` — 전체 테스트 (순차)
4. \`cd c5 && go test ./...\` — C5 Hub 테스트
5. 결과 요약: 패키지 수, 테스트 수, PASS/FAIL

오류 발견 시 수정하지 말고 보고만 하라.
" \
      --allowedTools "Read,Bash,Grep" \
      --model haiku \
      --print
    ;;

  review)
    FILE="${2:?Usage: review <file-path>}"
    unset CLAUDECODE
    claude -p "
파일 ${FILE}을 코드 리뷰하라.

우선순위:
1. 데이터 무결성 / 보안 / 권한
2. 장애 복구 가능성 (rollback, idempotency)
3. 에러 핸들링 누락
4. 동시성 안전성 (race condition, deadlock)
5. 테스트 커버리지 갭

각 발견 사항을 P0(critical) / P1(important) / P2(nice-to-have)로 분류.
수정 제안은 구체적 코드 스니펫으로 제시.
" \
      --allowedTools "$ALLOWED_RO" \
      --model opus \
      --print
    ;;

  migration-check)
    unset CLAUDECODE
    claude -p "
infra/supabase/migrations/ 디렉토리의 SQL 마이그레이션 파일을 검증하라.

1. 모든 .sql 파일을 읽고 테이블/컬럼 정의를 추출
2. 외래 키 참조가 올바른 테이블·컬럼을 가리키는지 확인
3. 타입 불일치 확인 (UUID vs TEXT, TIMESTAMPTZ vs TIMESTAMP 등)
4. RLS 정책에서 참조하는 컬럼이 실제로 존재하는지 확인
5. 발견된 문제를 파일명:줄번호와 함께 보고

수정하지 말고 보고만 하라.
" \
      --allowedTools "$ALLOWED_RO" \
      --model sonnet \
      --print
    ;;

  refactor)
    PLAN="${2:?Usage: refactor <plan-file-or-description>}"
    unset CLAUDECODE
    claude -p "
다음 리팩토링 계획을 실행하라: ${PLAN}

코드 작성 전:
1. Grep과 Read로 변경 대상 모듈을 import/참조하는 모든 파일을 매핑하라. 의존성 그래프를 만들어라.
2. 안전한 실행 순서를 결정하라 (leaf dependency 먼저).
3. 각 변경에 대해 순서대로:
   a) 수정 적용
   b) \`go build ./...\`로 컴파일 확인
   c) \`go vet ./...\`로 정적 분석
   d) 해당 패키지 테스트 실행
   실패 시 다음 단계 진행 전에 수정하라.
4. 전체 변경 후: 전체 테스트 실행, import cycle 확인, dead code 잔존 확인.
5. 전체 diff를 자기 리뷰: 순환 의존성, 스키마 불일치, 하드코딩 경로, 누락된 인터페이스 구현.
   발견 사항 수정.
6. 논리적 단위로 atomic 커밋 생성.
7. 최종 테스트 결과 + 변경 파일 요약 보고.
" \
      --allowedTools "$ALLOWED_RW" \
      --model sonnet \
      --print
    ;;

  explore)
    TOPIC="${2:?Usage: explore <topic-description>}"
    unset CLAUDECODE
    claude -p "
다음 기능/리팩토링에 대한 구현 계획을 작성하라: ${TOPIC}

코드는 작성하지 마라. 대신 다음을 수행:

1. CODEBASE MAPPING:
   - Glob과 Grep으로 관련 파일을 모두 찾아라
   - 각 파일의 구조(처음 50줄)를 읽어라
   - 의존성 맵 + 핵심 인터페이스/타입 목록을 만들어라

2. PATTERN ANALYSIS:
   - 이 코드베이스에서 이미 구현된 유사 기능 2-3개를 찾아라
   - 그 구현을 읽고 사용된 패턴, 컨벤션, 아키텍처 스타일을 문서화하라

3. RISK SCAN:
   - 잠재적 충돌: 깨질 수 있는 기존 테스트, 형성될 수 있는 import cycle,
     업데이트 필요한 설정, 필요한 마이그레이션 스크립트를 검색하라

4. 종합하여 다음을 포함하는 계획을 작성하라:
   a) 생성/수정 파일의 순서 목록 + 근거
   b) 먼저 정의할 핵심 인터페이스/타입
   c) 식별된 위험에 대한 대응 방안
   d) 테스트 전략
   e) 예상 커밋 수 + 그룹핑
" \
      --allowedTools "$ALLOWED_RO" \
      --model sonnet \
      --print
    ;;

  *)
    echo "Usage: $0 <command> [args...]"
    echo ""
    echo "Commands:"
    echo "  fix-test <package>     실패 테스트 자동 수정"
    echo "  verify                 전체 빌드 + 테스트 검증"
    echo "  review <file>          단일 파일 코드 리뷰"
    echo "  migration-check        SQL 마이그레이션 스키마 검증"
    echo "  refactor <plan>        계획 기반 다중 파일 리팩토링"
    echo "  explore <topic>        병렬 탐색 → 구현 계획 생성"
    ;;
esac
