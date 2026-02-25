---
name: c4-attach
description: "현재 Codex 세션에 이름을 연결해 재개 가능 상태로 저장"
triggers:
  - c4 attach
  - attach session
  - 세션 이름 붙여
---

# Goal
현재 세션에 재사용 가능한 이름을 붙이고 저장 상태를 확인합니다.

## Workflow
1. 이름 입력 확인:
   - 인자가 있으면 그대로 사용
   - 없으면 `basename $(pwd)`를 기본값으로 제안
2. `cq session name <name>` 실행.
3. `cq ls`로 `(current)` 표시와 저장 결과 확인.

## Safety Rules
- 이름이 비어 있으면 실행하지 않음.
- 기존 이름 충돌 시 덮어쓰기 여부를 사용자에게 확인.

## Output Checklist
- [ ] 설정한 세션 이름
- [ ] `cq session name` 결과
- [ ] `cq ls` 확인 결과
