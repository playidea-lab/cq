---
name: c4-release
description: "Git 히스토리 기반 CHANGELOG/버전 릴리스 준비"
triggers:
  - c4 release
  - release notes
  - 변경 로그
---

# Goal
최근 커밋을 분류해 릴리스 노트와 버전 판단 근거를 생성합니다.

## Workflow
1. 기준 태그 확인:
   - `git describe --tags --abbrev=0` (없으면 초기 릴리스로 처리)
2. 변경 커밋 수집:
   - 기준 태그 이후 `git log --pretty=format:"%h|%s|%an|%ad"`
3. Conventional Commit 분류:
   - `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`, `BREAKING CHANGE`
4. CHANGELOG 초안 생성/갱신.
5. 버전 bump 제안:
   - breaking -> major
   - feat -> minor
   - fix/chore 중심 -> patch
6. 사용자 확인 후 태그 생성 여부 결정.

## Safety Rules
- 태그 생성/푸시는 사용자 명시 승인 전 수행하지 않음.
- 커밋 이력이 비어 있으면 릴리스 중단 후 사유 보고.

## Output Checklist
- [ ] 분석 기간(태그/날짜)
- [ ] 카테고리별 커밋 수
- [ ] 제안 버전/다음 명령
