import { useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { CanvasData, ScanResult } from '../types';

export function useScanner() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<CanvasData | null>(null);

  const scanProject = useCallback(async (projectPath: string) => {
    setLoading(true);
    setError(null);

    try {
      const result = await invoke<ScanResult>('scan_project_cmd', {
        path: projectPath
      });

      if (result.success && result.data) {
        setData(result.data);
      } else {
        setError(result.error || 'Unknown error');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const saveCanvas = useCallback(async (projectPath: string, canvasData: CanvasData) => {
    try {
      await invoke('save_canvas', {
        path: projectPath,
        data: canvasData,
      });
    } catch (err) {
      console.error('Failed to save canvas:', err);
    }
  }, []);

  const loadCanvas = useCallback(async (projectPath: string) => {
    try {
      const result = await invoke<CanvasData | null>('load_canvas', {
        path: projectPath
      });
      if (result) {
        setData(result);
      }
      return result;
    } catch {
      return null;
    }
  }, []);

  return {
    loading,
    error,
    data,
    scanProject,
    saveCanvas,
    loadCanvas,
    setData,
  };
}
