feature: supabase-hub-migration
domain: go-backend
summary: fly.io Hub 서버 제거, c5 흡수, Supabase PostgREST + RPC + LISTEN/NOTIFY로 전환

requirements:
  ubiquitous:
    - "Worker는 Supabase LISTEN/NOTIFY를 통해 잡을 실시간으로 수신해야 한다"
    - "hub/client.go는 Supabase PostgREST를 통해 잡을 제출/조회해야 한다"
    - "잡 클레임은 PostgreSQL RPC function(FOR UPDATE SKIP LOCKED)으로 원자적이어야 한다"
    - "기존 MCP 도구(c4_hub_*, c4_worker_*) 인터페이스는 유지해야 한다"
    - "c5/store/sqlite.go의 20개 테이블이 Supabase PostgreSQL에 존재해야 한다"

  event_driven:
    - "WHEN 잡이 INSERT되면 THEN NOTIFY 트리거가 Worker에 알린다"
    - "WHEN 잡이 COMPLETE되면 THEN Edge Function이 텔레그램 알림을 전송한다"
    - "WHEN Worker가 온라인 되면 THEN LISTEN 연결을 수립하고 QUEUED 잡을 폴링한다"
    - "WHEN LISTEN 연결이 끊기면 THEN 자동 재연결하고 QUEUED 잡을 폴링한다"

  state_driven:
    - "WHILE Worker가 LISTEN 연결 중일 때 THEN 새 잡 즉시 수신"
    - "WHILE Worker LISTEN 연결이 끊겼을 때 THEN 재연결 후 QUEUED 잡 폴링"

  unwanted:
    - "fly.io 서버 의존"
    - "c5/ 독립 서버 바이너리"
    - "Supabase Realtime SDK 의존 (pgx 직접 연결로 충분)"

out_of_scope:
  - "C1 텔레그램 전용 재정의"
  - "MCP push dispatch 변경 (mcphttp 유지)"
  - "지식 시스템 변경 (이미 Supabase sync)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/hub/..."
    - "supabase db push (마이그레이션 적용)"
