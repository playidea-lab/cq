-- C9 Knowledge: Document Usage Tracking
-- Tracks view, search_hit, cite actions for popularity-based ranking.

CREATE TABLE IF NOT EXISTS c4_doc_usage (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    doc_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    user_id UUID REFERENCES auth.users(id),
    action TEXT NOT NULL CHECK (action IN ('view', 'search_hit', 'cite')),
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL
);

-- Indexes for efficient aggregation
CREATE INDEX idx_doc_usage_doc_id ON c4_doc_usage(doc_id);
CREATE INDEX idx_doc_usage_action ON c4_doc_usage(action);
CREATE INDEX idx_doc_usage_created ON c4_doc_usage(created_at DESC);

-- RLS: users can INSERT their own usage, anyone can SELECT aggregates
ALTER TABLE c4_doc_usage ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can record own usage"
    ON c4_doc_usage FOR INSERT
    TO authenticated
    WITH CHECK (user_id = auth.uid());

CREATE POLICY "Anyone can read usage aggregates"
    ON c4_doc_usage FOR SELECT
    TO authenticated
    USING (true);

-- Aggregate view for popularity scoring
CREATE OR REPLACE VIEW c4_doc_popularity AS
SELECT
    doc_id,
    COUNT(*) AS total_usage,
    COUNT(*) FILTER (WHERE action = 'view') AS view_count,
    COUNT(*) FILTER (WHERE action = 'search_hit') AS search_hit_count,
    COUNT(*) FILTER (WHERE action = 'cite') AS cite_count,
    -- Weighted popularity: cite=5, view=2, search_hit=1
    (COUNT(*) FILTER (WHERE action = 'cite') * 5 +
     COUNT(*) FILTER (WHERE action = 'view') * 2 +
     COUNT(*) FILTER (WHERE action = 'search_hit') * 1) AS popularity_score,
    MAX(created_at) AS last_used_at
FROM c4_doc_usage
GROUP BY doc_id;
