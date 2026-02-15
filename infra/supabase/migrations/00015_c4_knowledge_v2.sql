-- Migration: 00015_c4_knowledge_v2
-- C9 Knowledge System v2: pgvector embeddings, visibility, soft delete, user tracking

-- 1. Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. Add embedding column (1536 dims for text-embedding-3-small)
ALTER TABLE c4_documents ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- 3. Add visibility column (private/team/public)
ALTER TABLE c4_documents ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'team'
    CHECK (visibility IN ('private', 'team', 'public'));

-- 4. Add user tracking
ALTER TABLE c4_documents ADD COLUMN IF NOT EXISTS created_by_user_id UUID;

-- 5. Add soft delete support
ALTER TABLE c4_documents ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 6. Create HNSW index for vector similarity search
CREATE INDEX IF NOT EXISTS idx_c4_documents_embedding
    ON c4_documents USING hnsw (embedding vector_cosine_ops);

-- 7. Create index for visibility-based queries
CREATE INDEX IF NOT EXISTS idx_c4_documents_visibility
    ON c4_documents(visibility) WHERE deleted_at IS NULL;

-- 8. Create index for soft-delete-aware queries
CREATE INDEX IF NOT EXISTS idx_c4_documents_active
    ON c4_documents(project_id, updated_at DESC) WHERE deleted_at IS NULL;

-- ============================================================
-- RLS Policy Updates for visibility-based access control
-- ============================================================

-- Drop existing policies
DROP POLICY IF EXISTS "Members can view documents" ON c4_documents;
DROP POLICY IF EXISTS "Members can create documents" ON c4_documents;
DROP POLICY IF EXISTS "Members can update documents" ON c4_documents;
DROP POLICY IF EXISTS "Members can delete documents" ON c4_documents;

-- New visibility-aware SELECT policy:
-- - public: any authenticated user
-- - team: project members only
-- - private: document creator only
CREATE POLICY "Visibility-aware document access"
    ON c4_documents FOR SELECT
    USING (
        deleted_at IS NULL
        AND (
            visibility = 'public' AND auth.uid() IS NOT NULL
            OR visibility = 'team' AND c4_is_project_member(project_id)
            OR visibility = 'private' AND created_by_user_id = auth.uid()
            -- Legacy: documents without created_by_user_id accessible by project members
            OR created_by_user_id IS NULL AND c4_is_project_member(project_id)
        )
    );

-- INSERT: project members can create documents
CREATE POLICY "Members can create documents v2"
    ON c4_documents FOR INSERT
    WITH CHECK (c4_is_project_member(project_id));

-- UPDATE: only creator or project member can update
CREATE POLICY "Members can update documents v2"
    ON c4_documents FOR UPDATE
    USING (
        c4_is_project_member(project_id)
        AND (created_by_user_id IS NULL OR created_by_user_id = auth.uid())
    )
    WITH CHECK (c4_is_project_member(project_id));

-- DELETE: only creator or project member can delete
CREATE POLICY "Members can delete documents v2"
    ON c4_documents FOR DELETE
    USING (
        c4_is_project_member(project_id)
        AND (created_by_user_id IS NULL OR created_by_user_id = auth.uid())
    );

-- ============================================================
-- Function: semantic search via pgvector
-- ============================================================
CREATE OR REPLACE FUNCTION c4_knowledge_search_semantic(
    query_embedding vector(1536),
    match_count int DEFAULT 10,
    similarity_threshold float DEFAULT 0.5,
    filter_visibility text DEFAULT NULL,
    filter_project_id uuid DEFAULT NULL
)
RETURNS TABLE (
    doc_id text,
    project_id uuid,
    title text,
    doc_type text,
    domain text,
    visibility text,
    similarity float
)
LANGUAGE plpgsql
AS $$
BEGIN
    RETURN QUERY
    SELECT
        d.doc_id,
        d.project_id,
        d.title,
        d.doc_type,
        d.domain,
        d.visibility,
        1 - (d.embedding <=> query_embedding) AS similarity
    FROM c4_documents d
    WHERE d.deleted_at IS NULL
        AND d.embedding IS NOT NULL
        AND (filter_visibility IS NULL OR d.visibility = filter_visibility)
        AND (filter_project_id IS NULL OR d.project_id = filter_project_id)
        AND 1 - (d.embedding <=> query_embedding) > similarity_threshold
    ORDER BY d.embedding <=> query_embedding
    LIMIT match_count;
END;
$$;
