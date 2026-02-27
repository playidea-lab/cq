-- Migration: 00026_knowledge_vectors_768dim
-- Migrate knowledge_vectors embedding column from 1536-dim to 768-dim
-- (text-embedding-3-small default output dimension change)
--
-- IMPORTANT: Apply this migration ONLY after local reindex with 768-dim
-- embeddings is confirmed successful (T-QM-004b-0 prerequisite).
-- Rollback strategy: restore from backup taken before applying this migration.
--
-- WARNING: This drops and recreates the HNSW index and alters the column type.
-- Existing 1536-dim embedding data will be lost.
-- Run local reindex (T-QM-004b-0) to repopulate with 768-dim embeddings.

-- 1. Drop existing HNSW index (required before column type change)
DROP INDEX IF EXISTS idx_c4_documents_embedding;

-- 2. Drop old 1536-dim embedding column
ALTER TABLE c4_documents DROP COLUMN IF EXISTS embedding;

-- 3. Add new 768-dim embedding column
ALTER TABLE c4_documents ADD COLUMN embedding vector(768);

-- 4. Recreate HNSW index for 768-dim vectors
CREATE INDEX idx_c4_documents_embedding
    ON c4_documents USING hnsw (embedding vector_cosine_ops);

-- 5. Update semantic search function to use 768-dim
DROP FUNCTION IF EXISTS c4_knowledge_search_semantic(vector(1536), int, float, text, uuid);

CREATE OR REPLACE FUNCTION c4_knowledge_search_semantic(
    query_embedding vector(768),
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
