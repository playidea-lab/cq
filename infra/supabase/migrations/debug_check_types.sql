-- ============================================================================
-- 타입 불일치 진단 스크립트
-- Supabase SQL Editor에서 먼저 실행하여 문제를 파악하세요.
-- ============================================================================

-- 1. 기존 테이블들의 user_id, team_id 컬럼 타입 확인
SELECT
    table_name,
    column_name,
    data_type,
    udt_name
FROM information_schema.columns
WHERE column_name IN ('user_id', 'team_id', 'owner_id', 'actor_id', 'provider_id')
  AND table_schema = 'public'
ORDER BY table_name, column_name;

-- 2. UUID vs TEXT 불일치 찾기
SELECT
    table_name,
    column_name,
    data_type,
    udt_name,
    CASE
        WHEN column_name IN ('user_id', 'team_id', 'owner_id', 'project_id') AND udt_name != 'uuid'
        THEN '⚠️ 예상: uuid, 실제: ' || udt_name
        ELSE '✅ OK'
    END as status
FROM information_schema.columns
WHERE column_name IN ('user_id', 'team_id', 'owner_id', 'project_id', 'actor_id', 'provider_id')
  AND table_schema = 'public'
ORDER BY status DESC, table_name;

-- 3. 기존 RLS 정책 확인
SELECT
    schemaname,
    tablename,
    policyname,
    permissive,
    roles,
    cmd,
    qual
FROM pg_policies
WHERE schemaname = 'public'
ORDER BY tablename, policyname;
