import { useAuth } from '../../hooks/useAuth';
import '../../styles/auth.css';

export function LoginView() {
  const { loggingIn, error, config, login } = useAuth();

  const isConfigured = config?.supabase_url && config?.has_anon_key;

  return (
    <div className="login">
      <div className="login__card">
        <h1 className="login__title">C1</h1>
        <p className="login__subtitle">Multi-LLM Project Explorer</p>

        {!isConfigured ? (
          <div className="login__setup">
            <p className="login__setup-heading">Supabase configuration required</p>
            <div className="login__setup-instructions">
              <p>Set the following environment variables:</p>
              <code className="login__code">
                export SUPABASE_URL=https://your-project.supabase.co{'\n'}
                export SUPABASE_ANON_KEY=your-anon-key
              </code>
              <p className="login__setup-hint">
                Or add <code>supabase_url</code> to <code>~/.c4/cloud.yaml</code>
              </p>
            </div>
          </div>
        ) : (
          <div className="login__actions">
            <button
              className="login__btn"
              onClick={login}
              disabled={loggingIn}
            >
              {loggingIn ? (
                <>
                  <span className="login__spinner" />
                  Signing in...
                </>
              ) : (
                <>
                  <svg className="login__github-icon" viewBox="0 0 16 16" width="18" height="18">
                    <path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
                  </svg>
                  Sign in with GitHub
                </>
              )}
            </button>
          </div>
        )}

        {error && (
          <div className="login__error">
            <p>{error}</p>
            <button className="login__retry" onClick={login}>
              Try again
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
