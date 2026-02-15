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

  *)
    echo "Usage: $0 <command> [args...]"
    echo ""
    echo "Commands:"
    echo "  fix-test <package>     실패 테스트 자동 수정"
    echo "  verify                 전체 빌드 + 테스트 검증"
    echo "  review <file>          단일 파일 코드 리뷰"
    echo "  migration-check        SQL 마이그레이션 스키마 검증"
    ;;
esac
