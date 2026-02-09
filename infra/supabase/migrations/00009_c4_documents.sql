-- Migration: 00009_c4_documents
-- Knowledge Store cloud sync. Stores document metadata + body with PostgreSQL FTS.

CREATE TABLE c4_documents (
    doc_id       TEXT        NOT NULL,
    project_id   UUID        NOT NULL REFERENCES c4_projects(id) ON DELETE CASCADE,
    doc_type     TEXT        NOT NULL CHECK (doc_type IN ('experiment', 'pattern', 'insight', 'hypothesis')),
    title        TEXT        NOT NULL DEFAULT '',
    domain       TEXT        NOT NULL DEFAULT '',
    tags         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    body         TEXT        NOT NULL DEFAULT '',
    metadata     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    content_hash TEXT        NOT NULL DEFAULT '',
    version      INTEGER     NOT NULL DEFAULT 1,
    created_by   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (project_id, doc_id)
);

-- Full-text search: weighted tsvector (title=A, domain=B, body=C)
ALTER TABLE c4_documents ADD COLUMN tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(domain, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(body, '')), 'C')
    ) STORED;

CREATE INDEX idx_c4_documents_type    ON c4_documents(project_id, doc_type);
CREATE INDEX idx_c4_documents_domain  ON c4_documents(project_id, domain);
CREATE INDEX idx_c4_documents_updated ON c4_documents(project_id, updated_at DESC);
CREATE INDEX idx_c4_documents_fts     ON c4_documents USING GIN(tsv);

-- Auto-update updated_at
CREATE TRIGGER trg_c4_documents_updated_at
    BEFORE UPDATE ON c4_documents
    FOR EACH ROW
    EXECUTE FUNCTION c4_set_updated_at();

-- ============================================================
-- RLS
-- ============================================================
ALTER TABLE c4_documents ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Members can view documents"
    ON c4_documents FOR SELECT
    USING (c4_is_project_member(project_id));

CREATE POLICY "Members can create documents"
    ON c4_documents FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can update documents"
    ON c4_documents FOR UPDATE
    USING (c4_is_project_member(project_id))
    WITH CHECK (c4_is_project_member(project_id));

CREATE POLICY "Members can delete documents"
    ON c4_documents FOR DELETE
    USING (c4_is_project_member(project_id));
