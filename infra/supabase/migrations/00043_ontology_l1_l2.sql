-- Migration: 00043_ontology_l1_l2
-- Creates L1 (user) and L2 (project) ontology tables.
-- Stores the entire ontology schema as JSONB (L3 is collective_patterns, already in 00041).
-- RLS: L1 = user owns their own row; L2 = project member via JWT claim.

-- ============================================================
-- L1: c4_user_ontology
-- ============================================================
CREATE TABLE IF NOT EXISTS c4_user_ontology (
    user_id     UUID        PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    schema_jsonb JSONB      NOT NULL DEFAULT '{}',
    version     INT         NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE c4_user_ontology ENABLE ROW LEVEL SECURITY;

CREATE POLICY "c4_user_ontology: owner full access"
    ON c4_user_ontology FOR ALL
    USING (user_id = auth.uid())
    WITH CHECK (user_id = auth.uid());

-- ============================================================
-- L2: c4_project_ontology
-- ============================================================
CREATE TABLE IF NOT EXISTS c4_project_ontology (
    project_id  UUID        PRIMARY KEY,
    schema_jsonb JSONB      NOT NULL DEFAULT '{}',
    version     INT         NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE c4_project_ontology ENABLE ROW LEVEL SECURITY;

CREATE POLICY "c4_project_ontology: service_role full access"
    ON c4_project_ontology FOR ALL
    USING (auth.role() = 'service_role')
    WITH CHECK (auth.role() = 'service_role');

CREATE POLICY "c4_project_ontology: members can view own project"
    ON c4_project_ontology FOR SELECT
    USING (project_id::text = hub_jwt_project_id() AND hub_jwt_project_id() != '');
