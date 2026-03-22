-- Migration: 00041_collective_patterns
-- Creates collective_patterns table for L3 cluster intelligence data storage.
-- Stores shared behavioral patterns discovered across agent sessions.

CREATE TABLE collective_patterns (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    domain            TEXT        NOT NULL,
    path              TEXT        NOT NULL,
    value             TEXT        NOT NULL,
    frequency         INT         NOT NULL DEFAULT 1,
    confidence        TEXT        NOT NULL,
    tags              TEXT[]      NOT NULL DEFAULT '{}',
    contributor_count INT         NOT NULL DEFAULT 1,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT collective_patterns_domain_path_value_key UNIQUE (domain, path, value)
);

CREATE INDEX idx_collective_patterns_domain ON collective_patterns(domain);
CREATE INDEX idx_collective_patterns_tags   ON collective_patterns USING GIN(tags);

-- ============================================================
-- RLS: collective_patterns
-- 읽기=인증 유저, 쓰기=인증 유저
-- ============================================================
ALTER TABLE collective_patterns ENABLE ROW LEVEL SECURITY;

CREATE POLICY "collective_patterns: authenticated read"
    ON collective_patterns FOR SELECT
    USING (auth.role() = 'authenticated');

CREATE POLICY "collective_patterns: authenticated insert"
    ON collective_patterns FOR INSERT
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "collective_patterns: authenticated update"
    ON collective_patterns FOR UPDATE
    USING (auth.role() = 'authenticated')
    WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "collective_patterns: authenticated delete"
    ON collective_patterns FOR DELETE
    USING (auth.role() = 'authenticated');
