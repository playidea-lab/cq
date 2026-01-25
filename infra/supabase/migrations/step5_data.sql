-- ============================================================================
-- Step 5: 초기 데이터
-- Step 4 성공 후 실행하세요.
-- ============================================================================

-- ============================================================================
-- Integration Providers 초기 데이터
-- ============================================================================

INSERT INTO integration_providers (id, name, category, oauth_url, webhook_path, enabled)
VALUES
    ('github', 'GitHub', 'source_control', 'https://github.com/login/oauth/authorize', '/webhooks/github', true),
    ('discord', 'Discord', 'messaging', 'https://discord.com/api/oauth2/authorize', '/webhooks/discord', true),
    ('slack', 'Slack', 'messaging', 'https://slack.com/oauth/v2/authorize', '/webhooks/slack', true),
    ('dooray', 'Dooray', 'collaboration', NULL, '/webhooks/dooray', true),
    ('gitlab', 'GitLab', 'source_control', 'https://gitlab.com/oauth/authorize', '/webhooks/gitlab', false),
    ('bitbucket', 'Bitbucket', 'source_control', 'https://bitbucket.org/site/oauth2/authorize', '/webhooks/bitbucket', false)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    category = EXCLUDED.category,
    oauth_url = EXCLUDED.oauth_url,
    webhook_path = EXCLUDED.webhook_path,
    enabled = EXCLUDED.enabled;

SELECT '✅ Step 5 완료: 초기 데이터 삽입 성공!' as result;

-- ============================================================================
-- 전체 마이그레이션 완료!
-- ============================================================================

SELECT '🎉 전체 마이그레이션 완료!' as final_result;
