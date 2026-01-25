-- ============================================================================
-- role 체크 제약 수정
-- seed 실행 전에 먼저 실행하세요!
-- ============================================================================

-- 현재 제약 확인
SELECT
    tc.constraint_name,
    cc.check_clause
FROM information_schema.table_constraints tc
JOIN information_schema.check_constraints cc
    ON tc.constraint_name = cc.constraint_name
WHERE tc.table_name = 'team_members'
  AND tc.constraint_type = 'CHECK';

-- 기존 제약 삭제
ALTER TABLE team_members DROP CONSTRAINT IF EXISTS team_members_role_check;

-- 새 제약 추가 (owner, admin, member, viewer 허용)
ALTER TABLE team_members
ADD CONSTRAINT team_members_role_check
CHECK (role IN ('owner', 'admin', 'member', 'viewer'));

SELECT '✅ role 체크 제약 수정 완료! 이제 seed_playidealab.sql을 실행하세요.' as result;
