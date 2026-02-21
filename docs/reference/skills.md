# Skills Reference

Skills are slash commands invoked inside Claude Code. They are defined in `.claude/skills/` and embedded in the CQ binary (`skills_embed` build tag).

## Full list

| Skill | Triggers | Tier |
|-------|----------|------|
| `/c4-plan` | plan, 계획, 설계, 기획 | all |
| `/c4-run` | run, 실행, ㄱㄱ | all |
| `/c4-finish` | finish, 마무리, 완료 | all |
| `/c4-status` | status, 상태 | all |
| `/c4-quick` | quick, 빠르게 | all |
| `/c4-polish` | polish | all |
| `/c4-refine` | refine | all |
| `/c4-checkpoint` | (automatic at checkpoint) | all |
| `/c4-validate` | validate, 검증 | all |
| `/c4-add-task` | add task, 태스크 추가 | all |
| `/c4-submit` | submit, 제출 | all |
| `/c4-review` | review | all |
| `/c4-stop` | stop, 중단 | all |
| `/c4-help` | help | all |
| `/c4-swarm` | swarm | all |
| `/c4-standby` | standby, 대기, worker mode | full |
| `/c4-release` | release | all |
| `/c4-init` | init, 초기화 | all |
| `/c4-interview` | interview | all |

## Machine-readable

Download as JSONL:

```sh
curl https://playidealab.github.io/cq/api/skills.jsonl
```
