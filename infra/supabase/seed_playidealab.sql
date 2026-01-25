-- 플레이아이디어랩 테스트 데이터 시드
-- 사용법: Supabase Dashboard > SQL Editor에서 실행

-- ============================================================================
-- 사전 준비: Supabase Dashboard에서 사용자 먼저 생성하세요!
-- ============================================================================
-- 1. Supabase Dashboard > Authentication > Users
-- 2. "Add User" 클릭
-- 3. 이메일: admin@playidealab.com (또는 원하는 이메일)
-- 4. 비밀번호: 설정
-- 5. "Create User" 클릭
-- ============================================================================

-- 1. 사용자 ID 확인 (먼저 실행하여 사용자 존재 확인)
-- SELECT id, email FROM auth.users LIMIT 5;

-- ============================================================================
-- 2. 변수 설정
-- ============================================================================
DO $$
DECLARE
    v_owner_id UUID;
    v_team_id UUID;
    v_user_count INTEGER;
BEGIN
    -- 사용자 수 확인
    SELECT COUNT(*) INTO v_user_count FROM auth.users;

    -- 사용자가 없으면 안내 메시지와 함께 종료
    IF v_user_count = 0 THEN
        RAISE NOTICE '========================================';
        RAISE NOTICE '⚠️  사용자가 없습니다!';
        RAISE NOTICE '';
        RAISE NOTICE '다음 단계를 따라주세요:';
        RAISE NOTICE '1. Supabase Dashboard 접속';
        RAISE NOTICE '2. Authentication > Users 탭';
        RAISE NOTICE '3. "Add User" 클릭';
        RAISE NOTICE '4. 이메일/비밀번호 입력 후 생성';
        RAISE NOTICE '5. 이 SQL 다시 실행';
        RAISE NOTICE '========================================';
        RAISE EXCEPTION '사용자를 먼저 생성하세요. (Supabase Dashboard > Authentication > Users > Add User)';
    END IF;

    -- 첫 번째 사용자를 owner로 사용
    v_owner_id := (SELECT id FROM auth.users ORDER BY created_at LIMIT 1);

    RAISE NOTICE '사용자 발견: % (총 %명)', v_owner_id, v_user_count;

    -- ============================================================================
    -- 3. 팀 생성
    -- ============================================================================
    INSERT INTO teams (name, slug, owner_id, settings)
    VALUES (
        '플레이아이디어랩',
        'playidealab',
        v_owner_id,
        '{"plan": "team", "features": ["branding", "custom_domain"]}'::jsonb
    )
    ON CONFLICT (slug) DO UPDATE SET
        name = EXCLUDED.name,
        settings = EXCLUDED.settings
    RETURNING id INTO v_team_id;

    RAISE NOTICE '팀 생성됨: % (ID: %)', '플레이아이디어랩', v_team_id;

    -- ============================================================================
    -- 4. 팀 멤버 추가 (owner)
    -- ============================================================================
    INSERT INTO team_members (team_id, user_id, role)
    VALUES (v_team_id, v_owner_id, 'owner')
    ON CONFLICT (team_id, user_id) DO UPDATE SET
        role = 'owner';

    RAISE NOTICE '팀 멤버 추가됨: owner';

    -- ============================================================================
    -- 5. 브랜딩 설정
    -- ============================================================================
    INSERT INTO team_branding (
        team_id,
        brand_name,
        logo_url,
        logo_dark_url,
        favicon_url,
        primary_color,
        secondary_color,
        accent_color,
        background_color,
        text_color,
        heading_font,
        body_font,
        font_scale,
        email_from_name,
        email_footer_text,
        meta_description,
        hide_powered_by,
        created_by
    )
    VALUES (
        v_team_id,
        '플레이아이디어랩',
        'https://playidealab.com/logo.png',
        'https://playidealab.com/logo-dark.png',
        'https://playidealab.com/favicon.ico',
        '#2563EB',      -- Primary: Blue
        '#64748B',      -- Secondary: Slate
        '#F59E0B',      -- Accent: Amber
        '#FFFFFF',      -- Background: White
        '#1F2937',      -- Text: Gray 800
        'Pretendard',   -- Heading font
        'Pretendard',   -- Body font
        1.0,            -- Font scale
        '플레이아이디어랩',
        '© 2025 플레이아이디어랩. All rights reserved.',
        'AI 기반 소프트웨어 개발 자동화 플랫폼',
        false,
        v_owner_id
    )
    ON CONFLICT (team_id) DO UPDATE SET
        brand_name = EXCLUDED.brand_name,
        logo_url = EXCLUDED.logo_url,
        primary_color = EXCLUDED.primary_color,
        updated_at = NOW();

    RAISE NOTICE '브랜딩 설정 완료';

    -- ============================================================================
    -- 6. 결과 확인
    -- ============================================================================
    RAISE NOTICE '========================================';
    RAISE NOTICE '플레이아이디어랩 테스트 데이터 생성 완료!';
    RAISE NOTICE '팀 ID: %', v_team_id;
    RAISE NOTICE '========================================';

END $$;

-- ============================================================================
-- 7. 생성된 데이터 확인
-- ============================================================================
SELECT
    t.id as team_id,
    t.name as team_name,
    t.slug,
    tb.brand_name,
    tb.primary_color,
    tb.heading_font
FROM teams t
LEFT JOIN team_branding tb ON t.id = tb.team_id
WHERE t.slug = 'playidealab';
