import { createContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import type { AuthUser, AuthConfig } from '../types';

export interface AuthContextValue {
  user: AuthUser | null;
  loading: boolean;
  loggingIn: boolean;
  error: string | null;
  config: AuthConfig | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextValue>({
  user: null,
  loading: true,
  loggingIn: false,
  error: null,
  config: null,
  login: async () => {},
  logout: async () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [loggingIn, setLoggingIn] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [config, setConfig] = useState<AuthConfig | null>(null);

  // Load existing session + config on mount
  useEffect(() => {
    Promise.all([
      invoke<AuthUser | null>('auth_get_session').catch(() => null),
      invoke<AuthConfig>('auth_get_config').catch(() => null),
    ]).then(([u, c]) => {
      setUser(u ?? null);
      setConfig(c ?? null);
    }).finally(() => setLoading(false));
  }, []);

  // Listen for auth-changed events from Rust backend
  useEffect(() => {
    const unlisten = listen<AuthUser | null>('auth-changed', (event) => {
      setUser(event.payload ?? null);
    });
    return () => {
      unlisten.then((fn) => fn());
    };
  }, []);

  const login = useCallback(async () => {
    setError(null);
    setLoggingIn(true);
    try {
      const cfg = config ?? await invoke<AuthConfig>('auth_get_config');
      if (!cfg.supabase_url || !cfg.has_anon_key) {
        setError('Supabase is not configured. Set SUPABASE_URL and SUPABASE_ANON_KEY environment variables.');
        return;
      }
      const u = await invoke<AuthUser>('auth_login', {
        supabaseUrl: cfg.supabase_url,
        anonKey: '__FROM_ENV__',
      });
      setUser(u);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoggingIn(false);
    }
  }, [config]);

  const logout = useCallback(async () => {
    try {
      await invoke('auth_logout');
      setUser(null);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  return (
    <AuthContext.Provider value={{ user, loading, loggingIn, error, config, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}
