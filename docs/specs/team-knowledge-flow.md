feature: team-knowledge-flow
domain: web-backend
summary: "설치 → 로그인 → clone → 작업하면 팀 지식이 알아서 흐른다"

requirements:
  - id: E1
    type: event-driven
    text: "WHEN 유저가 로그인 후 프로젝트 디렉토리에서 cq/claude를 실행하고, .c4/config.yaml에 invite_code가 있고 유저가 해당 project의 member가 아닌 경우, THE SYSTEM SHALL 자동으로 InviteOrAdd(project_id, user_email)를 호출하여 멤버로 등록한다."
  - id: E2
    type: event-driven
    text: "WHEN 유저가 c4_experiment_search를 호출하고 cloud-primary 모드인 경우, THE SYSTEM SHALL Supabase semantic search를 사용하여 같은 project의 team visibility 문서를 검색한다."
  - id: E3
    type: event-driven
    text: "WHEN 태스크가 워커에게 할당될 때 (enrichUnified), THE SYSTEM SHALL cloud knowledge search를 포함하여 Past Solutions를 구성한다."
  - id: E4
    type: unwanted
    text: "THE SYSTEM SHALL NOT API 키, project ID, invite code를 수동 공유하도록 요구한다."

non_functional:
  - cloud 미연결 시 로컬 fallback (기존 동작 유지)
  - cloud search timeout 2초, 실패 시 로컬만
  - invite_code 유출 시에도 Supabase 인증 필수

out_of_scope:
  - Level 2/3 (hub 가시성, dashboard, 팀 지능)
  - Web UI
  - role-based access control
