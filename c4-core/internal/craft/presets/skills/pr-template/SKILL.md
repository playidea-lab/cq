---
name: pr-template
description: |
  PR 생성 시 summary, test plan, checklist를 자동 생성. git log와 diff를 분석해 PR 본문 초안 작성.
  트리거: "PR 만들어줘", "pull request", "pr-template", "PR 본문 작성"
allowed-tools: Read, Bash, Glob, Grep
---
# PR Template

git log와 diff를 분석해 PR 본문을 자동으로 생성합니다.

## 실행 순서

### Step 1: 변경 내용 파악

```bash
# 현재 브랜치와 base 브랜치의 diff 요약
git log --oneline origin/main...HEAD
git diff --stat origin/main...HEAD
git diff origin/main...HEAD
```

### Step 2: PR 본문 생성

아래 템플릿에 분석 결과를 채워 출력한다.

```markdown
## Summary

<!-- 변경의 목적과 배경 (왜 이 PR이 필요한가) -->
- ...
- ...

## Changes

<!-- 주요 변경 파일과 내용 -->
| 파일 | 변경 내용 |
|------|----------|
| `path/to/file.go` | ... |

## Test Plan

<!-- 이 PR을 검증하는 방법 -->
- [ ] 단위 테스트 실행: `go test ./...`
- [ ] 수동 테스트 시나리오:
  1. ...
  2. ...

## Checklist

- [ ] 코드 리뷰 6축 자체 검토 완료
- [ ] 테스트 추가/업데이트
- [ ] 문서 업데이트 (필요 시)
- [ ] 환경변수/설정 변경사항 팀 공유
- [ ] 롤백 계획 수립 (DB 마이그레이션 포함 시)

## Screenshots (선택)

<!-- UI 변경이 있으면 before/after 스크린샷 -->
```

### Step 3: gh CLI로 PR 생성 (선택)

사용자가 원하면 아래 명령으로 직접 생성:

```bash
gh pr create \
  --title "<type>(<scope>): <subject>" \
  --body "$(cat <<'EOF'
<생성된 본문>
EOF
)"
```

# CUSTOMIZE: PR 체크리스트 항목 추가
# 팀/프로젝트에 맞게 아래 항목을 수정하세요:
# - [ ] 성능 테스트 결과 첨부
# - [ ] 보안 리뷰 완료
# - [ ] 디자인 리뷰 승인
# - [ ] 번역/i18n 업데이트
