-- ============================================================================
-- 누락된 컬럼 추가 스크립트
-- step2 실행 전에 먼저 실행하세요!
-- ============================================================================

-- ============================================================================
-- teams 테이블 컬럼 추가
-- ============================================================================

-- owner_id
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'teams' AND column_name = 'owner_id' AND table_schema = 'public'
    ) THEN
        ALTER TABLE teams ADD COLUMN owner_id UUID;
        RAISE NOTICE 'teams.owner_id 컬럼 추가됨';
    ELSE
        RAISE NOTICE 'teams.owner_id 이미 존재';
    END IF;
END;
$$;

-- slug
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'teams' AND column_name = 'slug' AND table_schema = 'public'
    ) THEN
        ALTER TABLE teams ADD COLUMN slug TEXT UNIQUE;
        RAISE NOTICE 'teams.slug 컬럼 추가됨';
    ELSE
        RAISE NOTICE 'teams.slug 이미 존재';
    END IF;
END;
$$;

-- settings
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'teams' AND column_name = 'settings' AND table_schema = 'public'
    ) THEN
        ALTER TABLE teams ADD COLUMN settings JSONB DEFAULT '{}';
        RAISE NOTICE 'teams.settings 컬럼 추가됨';
    ELSE
        RAISE NOTICE 'teams.settings 이미 존재';
    END IF;
END;
$$;

-- stripe_customer_id
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'teams' AND column_name = 'stripe_customer_id' AND table_schema = 'public'
    ) THEN
        ALTER TABLE teams ADD COLUMN stripe_customer_id TEXT;
        RAISE NOTICE 'teams.stripe_customer_id 컬럼 추가됨';
    ELSE
        RAISE NOTICE 'teams.stripe_customer_id 이미 존재';
    END IF;
END;
$$;

-- plan
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'teams' AND column_name = 'plan' AND table_schema = 'public'
    ) THEN
        ALTER TABLE teams ADD COLUMN plan TEXT DEFAULT 'free';
        RAISE NOTICE 'teams.plan 컬럼 추가됨';
    ELSE
        RAISE NOTICE 'teams.plan 이미 존재';
    END IF;
END;
$$;

-- ============================================================================
-- team_members 테이블 컬럼 추가
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'invited_by' AND table_schema = 'public'
    ) THEN
        ALTER TABLE team_members ADD COLUMN invited_by UUID;
        RAISE NOTICE 'team_members.invited_by 컬럼 추가됨';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'invited_at' AND table_schema = 'public'
    ) THEN
        ALTER TABLE team_members ADD COLUMN invited_at TIMESTAMPTZ;
        RAISE NOTICE 'team_members.invited_at 컬럼 추가됨';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'accepted_at' AND table_schema = 'public'
    ) THEN
        ALTER TABLE team_members ADD COLUMN accepted_at TIMESTAMPTZ;
        RAISE NOTICE 'team_members.accepted_at 컬럼 추가됨';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'invite_email' AND table_schema = 'public'
    ) THEN
        ALTER TABLE team_members ADD COLUMN invite_email TEXT;
        RAISE NOTICE 'team_members.invite_email 컬럼 추가됨';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'team_members' AND column_name = 'invite_token' AND table_schema = 'public'
    ) THEN
        ALTER TABLE team_members ADD COLUMN invite_token TEXT;
        RAISE NOTICE 'team_members.invite_token 컬럼 추가됨';
    END IF;
END;
$$;

-- ============================================================================
-- projects 테이블 컬럼 확인/추가
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projects' AND column_name = 'created_by' AND table_schema = 'public'
    ) THEN
        ALTER TABLE projects ADD COLUMN created_by UUID;
        RAISE NOTICE 'projects.created_by 컬럼 추가됨';
    END IF;
END;
$$;

-- ============================================================================
-- 현재 테이블 구조 확인
-- ============================================================================

SELECT
    table_name,
    string_agg(column_name, ', ' ORDER BY ordinal_position) as columns
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name IN ('teams', 'team_members', 'projects')
GROUP BY table_name
ORDER BY table_name;

SELECT '✅ 누락된 컬럼 추가 완료! 이제 step2를 다시 실행하세요.' as result;
