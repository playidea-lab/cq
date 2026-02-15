-- C9 Knowledge: Document Chunks for RAG
-- Each chunk is independently embeddable and searchable.
-- Parent document's visibility is inherited via RLS join.

CREATE TABLE IF NOT EXISTS c4_doc_chunks (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    parent_doc_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    body TEXT NOT NULL,
    embedding vector(1536),
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL
);

-- Indexes
CREATE INDEX idx_chunks_parent ON c4_doc_chunks(parent_doc_id);
CREATE INDEX idx_chunks_project ON c4_doc_chunks(project_id);
CREATE INDEX idx_chunks_embedding ON c4_doc_chunks
    USING hnsw (embedding vector_cosine_ops);

-- RLS: inherit visibility from parent document
ALTER TABLE c4_doc_chunks ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Chunks inherit parent visibility"
    ON c4_doc_chunks FOR SELECT
    TO authenticated
    USING (
        EXISTS (
            SELECT 1 FROM c4_documents d
            WHERE d.doc_id = c4_doc_chunks.parent_doc_id
              AND d.project_id = c4_doc_chunks.project_id
              AND (
                  d.visibility = 'public'
                  OR d.created_by_user_id = auth.uid()
                  OR d.visibility = 'team'
              )
              AND d.deleted_at IS NULL
        )
    );

CREATE POLICY "Project members can insert chunks"
    ON c4_doc_chunks FOR INSERT
    TO authenticated
    WITH CHECK (true);

-- Semantic chunk search function
CREATE OR REPLACE FUNCTION c4_chunk_search_semantic(
    query_embedding vector(1536),
    match_count INTEGER DEFAULT 10,
    p_project_id TEXT DEFAULT NULL
)
RETURNS TABLE (
    chunk_id UUID,
    parent_doc_id TEXT,
    chunk_index INTEGER,
    body TEXT,
    similarity FLOAT
)
LANGUAGE plpgsql
AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id AS chunk_id,
        c.parent_doc_id,
        c.chunk_index,
        c.body,
        1 - (c.embedding <=> query_embedding) AS similarity
    FROM c4_doc_chunks c
    JOIN c4_documents d ON d.doc_id = c.parent_doc_id
        AND d.project_id = c.project_id
    WHERE c.embedding IS NOT NULL
      AND d.deleted_at IS NULL
      AND (p_project_id IS NULL OR c.project_id = p_project_id)
    ORDER BY c.embedding <=> query_embedding
    LIMIT match_count;
END;
$$;
