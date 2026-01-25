-- ============================================================================
-- 컬럼 타입 수정 스크립트
-- step2 실행 전에 먼저 실행하세요!
-- ============================================================================

-- 먼저 기존 FK 제약 제거 (있으면)
ALTER TABLE c4_state DROP CONSTRAINT IF EXISTS c4_state_project_id_fkey;
ALTER TABLE c4_tasks DROP CONSTRAINT IF EXISTS c4_tasks_state_id_fkey;
ALTER TABLE c4_workers DROP CONSTRAINT IF EXISTS c4_workers_state_id_fkey;
ALTER TABLE c4_events DROP CONSTRAINT IF EXISTS c4_events_state_id_fkey;

-- ============================================================================
-- c4_state.project_id: TEXT → UUID
-- ============================================================================
DO $$
BEGIN
    -- 데이터가 있으면 변환 시도, 없으면 그냥 타입 변경
    IF EXISTS (SELECT 1 FROM c4_state LIMIT 1) THEN
        ALTER TABLE c4_state
        ALTER COLUMN project_id TYPE UUID USING project_id::UUID;
        RAISE NOTICE 'c4_state.project_id: TEXT → UUID 변환 완료 (기존 데이터 유지)';
    ELSE
        ALTER TABLE c4_state
        ALTER COLUMN project_id TYPE UUID USING project_id::UUID;
        RAISE NOTICE 'c4_state.project_id: TEXT → UUID 변환 완료 (빈 테이블)';
    END IF;
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'c4_state.project_id 변환 실패: %. 테이블을 재생성합니다.', SQLERRM;
        DROP TABLE IF EXISTS c4_events CASCADE;
        DROP TABLE IF EXISTS c4_workers CASCADE;
        DROP TABLE IF EXISTS c4_tasks CASCADE;
        DROP TABLE IF EXISTS c4_state CASCADE;

        CREATE TABLE c4_state (
            id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            project_id UUID NOT NULL,
            current_status TEXT NOT NULL DEFAULT 'INIT',
            current_phase TEXT,
            priority_queue TEXT[] DEFAULT '{}',
            completed_tasks TEXT[] DEFAULT '{}',
            failed_tasks TEXT[] DEFAULT '{}',
            checkpoint_results JSONB DEFAULT '{}',
            validation_results JSONB DEFAULT '{}',
            context JSONB DEFAULT '{}',
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE c4_tasks (
            id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            state_id UUID NOT NULL,
            task_id TEXT NOT NULL,
            title TEXT NOT NULL,
            description TEXT,
            scope TEXT,
            dod TEXT,
            priority INTEGER DEFAULT 0,
            dependencies TEXT[] DEFAULT '{}',
            status TEXT DEFAULT 'pending',
            assigned_worker TEXT,
            result JSONB,
            error TEXT,
            retries INTEGER DEFAULT 0,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE c4_workers (
            id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            state_id UUID NOT NULL,
            worker_id TEXT NOT NULL,
            status TEXT DEFAULT 'idle',
            current_task TEXT,
            started_at TIMESTAMPTZ,
            last_heartbeat TIMESTAMPTZ,
            metadata JSONB DEFAULT '{}'
        );

        CREATE TABLE c4_events (
            id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            state_id UUID NOT NULL,
            event_type TEXT NOT NULL,
            payload JSONB DEFAULT '{}',
            created_at TIMESTAMPTZ DEFAULT NOW()
        );

        RAISE NOTICE 'c4 관련 테이블 재생성 완료';
END;
$$;

-- ============================================================================
-- 다른 테이블들도 확인 (필요시)
-- ============================================================================

-- projects.team_id 확인
DO $$
DECLARE
    v_type TEXT;
BEGIN
    SELECT udt_name INTO v_type
    FROM information_schema.columns
    WHERE table_name = 'projects' AND column_name = 'team_id' AND table_schema = 'public';

    IF v_type = 'text' THEN
        ALTER TABLE projects ALTER COLUMN team_id TYPE UUID USING team_id::UUID;
        RAISE NOTICE 'projects.team_id: TEXT → UUID 변환 완료';
    ELSE
        RAISE NOTICE 'projects.team_id: 이미 UUID 타입';
    END IF;
END;
$$;

-- team_members.team_id 확인
DO $$
DECLARE
    v_type TEXT;
BEGIN
    SELECT udt_name INTO v_type
    FROM information_schema.columns
    WHERE table_name = 'team_members' AND column_name = 'team_id' AND table_schema = 'public';

    IF v_type = 'text' THEN
        ALTER TABLE team_members ALTER COLUMN team_id TYPE UUID USING team_id::UUID;
        RAISE NOTICE 'team_members.team_id: TEXT → UUID 변환 완료';
    ELSE
        RAISE NOTICE 'team_members.team_id: 이미 UUID 타입';
    END IF;
END;
$$;

-- team_members.user_id 확인
DO $$
DECLARE
    v_type TEXT;
BEGIN
    SELECT udt_name INTO v_type
    FROM information_schema.columns
    WHERE table_name = 'team_members' AND column_name = 'user_id' AND table_schema = 'public';

    IF v_type = 'text' THEN
        ALTER TABLE team_members ALTER COLUMN user_id TYPE UUID USING user_id::UUID;
        RAISE NOTICE 'team_members.user_id: TEXT → UUID 변환 완료';
    ELSE
        RAISE NOTICE 'team_members.user_id: 이미 UUID 타입';
    END IF;
END;
$$;

SELECT '✅ 컬럼 타입 수정 완료! 이제 step2를 다시 실행하세요.' as result;
