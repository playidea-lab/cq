-- Migration: 00057_hub_metrics_rls_authenticated
-- Allow authenticated users to INSERT and SELECT hub_metrics.
-- Worker uses cloud JWT (authenticated role) to log real-time @key=value metrics.

CREATE POLICY "hub_metrics: authenticated can insert"
    ON hub_metrics FOR INSERT
    TO authenticated
    WITH CHECK (true);

CREATE POLICY "hub_metrics: authenticated can select"
    ON hub_metrics FOR SELECT
    TO authenticated
    USING (true);
