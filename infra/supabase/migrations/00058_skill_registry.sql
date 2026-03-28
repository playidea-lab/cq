-- Migration: 00058_skill_registry
-- Skill Marketplace Registry: central registry for skills, agents, and rules.
-- Phase 1: curated (admin-only publish), anyone can read/install.

-- ============================================================
-- skill_registry: one row per skill (metadata only)
-- ============================================================
CREATE TABLE skill_registry (
    id              UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    type            TEXT NOT NULL CHECK (type IN ('skill', 'agent', 'rule', 'claude-md')),
    description     TEXT NOT NULL DEFAULT '',
    author_id       UUID REFERENCES auth.users(id),
    author_name     TEXT NOT NULL DEFAULT '',
    latest_version  TEXT NOT NULL DEFAULT '1.0.0',
    download_count  BIGINT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'rejected', 'deprecated')),
    tags            TEXT[] DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FTS index on name + description
CREATE INDEX idx_skill_registry_fts
    ON skill_registry USING gin(to_tsvector('english', name || ' ' || description));

CREATE INDEX idx_skill_registry_type   ON skill_registry(type);
CREATE INDEX idx_skill_registry_status ON skill_registry(status);

-- ============================================================
-- skill_registry_versions: version history with content
-- ============================================================
CREATE TABLE skill_registry_versions (
    id          UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    skill_id    UUID NOT NULL REFERENCES skill_registry(id) ON DELETE CASCADE,
    version     TEXT NOT NULL,
    content     TEXT NOT NULL,
    extra_files JSONB DEFAULT '{}',
    changelog   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(skill_id, version)
);

CREATE INDEX idx_skill_registry_versions_skill ON skill_registry_versions(skill_id);

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE skill_registry ENABLE ROW LEVEL SECURITY;
ALTER TABLE skill_registry_versions ENABLE ROW LEVEL SECURITY;

-- Anyone authenticated can read active skills
CREATE POLICY "skill_registry: anyone can read active"
    ON skill_registry FOR SELECT
    TO authenticated
    USING (status = 'active');

-- Anon can also read (for unauthenticated browsing)
CREATE POLICY "skill_registry: anon can read active"
    ON skill_registry FOR SELECT
    TO anon
    USING (status = 'active');

-- Admin-only write (Phase 1: curated)
CREATE POLICY "skill_registry: admin can insert"
    ON skill_registry FOR INSERT
    TO authenticated
    WITH CHECK (
        (auth.jwt()->'user_metadata'->>'is_admin')::boolean = true
    );

CREATE POLICY "skill_registry: admin can update"
    ON skill_registry FOR UPDATE
    TO authenticated
    USING (
        (auth.jwt()->'user_metadata'->>'is_admin')::boolean = true
    )
    WITH CHECK (
        (auth.jwt()->'user_metadata'->>'is_admin')::boolean = true
    );

-- Versions: anyone can read
CREATE POLICY "skill_registry_versions: anyone can read"
    ON skill_registry_versions FOR SELECT
    TO authenticated
    USING (true);

CREATE POLICY "skill_registry_versions: anon can read"
    ON skill_registry_versions FOR SELECT
    TO anon
    USING (true);

-- Versions: admin can write
CREATE POLICY "skill_registry_versions: admin can insert"
    ON skill_registry_versions FOR INSERT
    TO authenticated
    WITH CHECK (
        (auth.jwt()->'user_metadata'->>'is_admin')::boolean = true
    );

-- ============================================================
-- RPC: atomic download count increment
-- ============================================================
CREATE OR REPLACE FUNCTION increment_skill_download(p_skill_name TEXT)
RETURNS void
LANGUAGE sql
SECURITY DEFINER
AS $$
    UPDATE skill_registry
    SET download_count = download_count + 1,
        updated_at = now()
    WHERE name = p_skill_name
      AND status = 'active';
$$;

-- Grant execute to authenticated and anon (install doesn't require login)
GRANT EXECUTE ON FUNCTION increment_skill_download(TEXT) TO authenticated;
GRANT EXECUTE ON FUNCTION increment_skill_download(TEXT) TO anon;
