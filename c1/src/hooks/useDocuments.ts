import { useState, useCallback, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { DocType, DocumentMeta, DocumentContent } from '../types';

export function useDocuments(projectPath: string | null, docType: DocType) {
  const [documents, setDocuments] = useState<DocumentMeta[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchDocuments = useCallback(async () => {
    if (!projectPath) return;
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<DocumentMeta[]>('list_documents', {
        projectPath,
        docType,
      });
      setDocuments(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [projectPath, docType]);

  useEffect(() => {
    fetchDocuments();
  }, [fetchDocuments]);

  return { documents, loading, error, refresh: fetchDocuments };
}

export function useDocument(path: string | null) {
  const [doc, setDoc] = useState<DocumentContent | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const fetchDocument = useCallback(async () => {
    if (!path) {
      setDoc(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<DocumentContent>('get_document', { path });
      setDoc(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [path]);

  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  const save = useCallback(async (content: string) => {
    if (!path) return;
    setSaving(true);
    try {
      await invoke('save_document', { path, content });
      setDoc(prev => prev ? { ...prev, content } : null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }, [path]);

  return { doc, loading, error, saving, save, refresh: fetchDocument };
}
