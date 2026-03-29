-- Migration: 00059_profiles_tier
-- Adds tier column to c4_profiles for freemium/pro distinction.
-- Default 'free'. Pro users bypass LLM proxy rate limits.

ALTER TABLE c4_profiles
    ADD COLUMN tier TEXT NOT NULL DEFAULT 'free'
    CHECK (tier IN ('free', 'pro'));

COMMENT ON COLUMN c4_profiles.tier IS 'Subscription tier: free (100 calls/month) or pro (unlimited)';
