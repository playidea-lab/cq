# Supabase Configuration for C4 Cloud

C4 Cloud uses Supabase for authentication and real-time features.

## Setup Guide

### 1. Create Supabase Project

1. Go to [supabase.com](https://supabase.com) and create a new project
2. Note down:
   - Project URL (e.g., `https://xxxx.supabase.co`)
   - Anon Key (public)
   - Service Role Key (private, server-side only)

### 2. Configure Environment Variables

Copy the template and fill in your values:

```bash
cp .env.example .env
```

Required variables:

| Variable | Description | Where to find |
|----------|-------------|---------------|
| `SUPABASE_URL` | Project URL | Settings > API |
| `SUPABASE_ANON_KEY` | Public API key | Settings > API |
| `SUPABASE_SERVICE_KEY` | Private API key | Settings > API (Service Role) |

### 3. Configure Auth Providers

#### GitHub OAuth

1. Go to Supabase Dashboard > Authentication > Providers
2. Enable GitHub provider
3. Create GitHub OAuth App:
   - Go to GitHub > Settings > Developer settings > OAuth Apps
   - Homepage URL: `https://your-c4-domain.com`
   - Authorization callback URL: `https://xxxx.supabase.co/auth/v1/callback`
4. Copy Client ID and Secret to Supabase

#### Google OAuth

1. Go to Supabase Dashboard > Authentication > Providers
2. Enable Google provider
3. Create Google OAuth credentials:
   - Go to [Google Cloud Console](https://console.cloud.google.com)
   - Create OAuth 2.0 Client ID (Web application)
   - Add authorized redirect URI: `https://xxxx.supabase.co/auth/v1/callback`
4. Copy Client ID and Secret to Supabase

### 4. Database Schema

Run the migrations to set up the database schema:

```bash
supabase db push
```

Or manually execute the SQL in `migrations/` folder.

## Local Development

### Using Supabase CLI

```bash
# Install Supabase CLI
brew install supabase/tap/supabase

# Start local Supabase
supabase start

# Use local URLs in .env.local
SUPABASE_URL=http://localhost:54321
SUPABASE_ANON_KEY=<local-anon-key>
```

### Testing Auth Flow

```bash
# Test login flow
c4 login

# Verify session
c4 auth status
```

## Security Notes

- Never commit `.env` files with real keys
- Use `SUPABASE_SERVICE_KEY` only on server-side
- Enable Row Level Security (RLS) for all tables
- Use `SUPABASE_ANON_KEY` for client-side operations

## Files

- `.env.example` - Environment variable template
- `migrations/` - Database migrations
- `config.toml` - Supabase project configuration
