# Git Workflow

> 브랜치 전략, 커밋 메시지, PR 규칙.

## 브랜치 전략

- `main` — 항상 배포 가능 상태. 직접 push 금지.
- `feature/<설명>` — 기능 개발. main에서 분기, MR로 병합.
- `fix/<설명>` — 버그 수정.
- `hotfix/<설명>` — 긴급 수정. main에서 직접 분기.

## 커밋 메시지

Conventional Commits 형식:

```
<type>(<scope>): <subject>

<body>

<footer>
```

| type | 용도 |
|------|------|
| feat | 새 기능 |
| fix | 버그 수정 |
| docs | 문서 변경 |
| refactor | 리팩토링 (기능 변경 없음) |
| test | 테스트 추가/수정 |
| chore | 빌드, CI, 의존성 등 |
| perf | 성능 개선 |

- subject는 50자 이내, 명령형 ("Add X", not "Added X")
- body는 "왜" 변경했는지 설명

## MR(Merge Request) 규칙

- MR 제목은 커밋 메시지 형식과 동일.
- 설명에 "무엇을, 왜" 포함.
- 최소 1명 리뷰 승인 후 병합.
- CI 통과 필수.
- self-merge 가능하지만, 중요 변경은 타인 리뷰 권장.

## .gitignore 필수 항목

```
# 시크릿
.env
.env.*
*.pem
*.key
credentials.json

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db

# 빌드 산출물
dist/
build/
*.o
*.exe
```
