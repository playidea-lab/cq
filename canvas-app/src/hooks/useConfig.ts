import { useState, useCallback, useMemo } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ConfigFileEntry, ConfigFileContent, ConfigCategory } from '../types';

const CATEGORY_LABELS: Record<ConfigCategory, string> = {
  global: 'Global Settings',
  project: 'Project Rules',
  persona: 'Personas',
  c4: 'C4 Config',
  memory: 'Memory',
};

const CATEGORY_ORDER: ConfigCategory[] = ['global', 'project', 'persona', 'c4', 'memory'];

export function useConfig() {
  const [files, setFiles] = useState<ConfigFileEntry[]>([]);
  const [selectedFile, setSelectedFile] = useState<ConfigFileContent | null>(null);
  const [loading, setLoading] = useState(false);
  const [contentLoading, setContentLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadFiles = useCallback(async (projectPath: string) => {
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<ConfigFileEntry[]>('list_config_files', { path: projectPath });
      setFiles(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadContent = useCallback(async (filePath: string) => {
    setContentLoading(true);
    try {
      const result = await invoke<ConfigFileContent>('read_config_file', { filePath });
      setSelectedFile(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setContentLoading(false);
    }
  }, []);

  const grouped = useMemo(() => {
    const groups: Record<string, { label: string; files: ConfigFileEntry[] }> = {};
    for (const cat of CATEGORY_ORDER) {
      const catFiles = files.filter(f => f.category === cat);
      if (catFiles.length > 0) {
        groups[cat] = { label: CATEGORY_LABELS[cat], files: catFiles };
      }
    }
    return groups;
  }, [files]);

  return {
    files,
    grouped,
    selectedFile,
    loading,
    contentLoading,
    error,
    loadFiles,
    loadContent,
  };
}
