---
name: daily-standup
description: |
  어제/오늘/블로커 스탠드업 요약. git log 기반으로 어제 작업을 자동 수집하고 정리.
  트리거: "스탠드업", "standup", "daily", "어제 뭐 했는지 정리해줘"
allowed-tools: Bash, Read
---
# Daily Standup

git log를 기반으로 스탠드업 내용을 자동 생성합니다.

## 실행 순서

### Step 1: 어제 커밋 수집

```bash
# 어제 본인 커밋 조회
git log --oneline --since="yesterday 00:00" --until="today 00:00" --author="$(git config user.name)"

# 오늘 커밋 (있다면)
git log --oneline --since="today 00:00" --author="$(git config user.name)"

# 현재 브랜치 확인
git branch --show-current

# 미커밋 작업 확인
git status --short
```

### Step 2: 스탠드업 초안 생성

```
## Daily Standup — YYYY-MM-DD

**어제 한 일**
- [커밋 메시지 기반으로 자동 요약]
- ...

**오늘 할 일**
- [현재 브랜치/미커밋 작업 기반 추론]
- ...

**블로커**
- 없음 (있으면 명시)
```

### Step 3: 사용자 검토 및 보완

초안을 제시하고 블로커나 추가 컨텍스트를 물어본다.

- "블로커 있으신가요?"
- "오늘 계획 추가할 항목 있으신가요?"

## 출력 형식

슬랙/팀즈에 바로 붙여넣을 수 있는 텍스트로 출력한다.

```
[스탠드업 YYYY-MM-DD]

✅ 어제:
• ...

🎯 오늘:
• ...

🚧 블로커:
• 없음
```

# CUSTOMIZE: 출력 포맷 변경
# 팀 채널에 맞게 이모지와 형식을 수정하세요.
# 슬랙 멘션이 필요하면 @channel, @here 추가.
# 특정 프로젝트 키(Jira, Linear 등) 패턴 추출 원하면 regex 추가:
# JIRA_PATTERN = r'[A-Z]+-\d+'
