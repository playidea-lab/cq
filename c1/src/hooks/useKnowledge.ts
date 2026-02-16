import { useState, useCallback, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';

export interface KnowledgeItem {
  id: string;
  doc_type: string;
  title: string;
  domain: string;
  tags: string[];
  created_at: string;
  updated_at: string;
  version: number;
}

export interface KnowledgeDocument {
  id: string;
  doc_type: string;
  title: string;
  domain: string;
  tags: string[];
  body: string;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface KnowledgeStats {
  total_documents: number;
  by_type: { doc_type: string; count: number }[];
}

export function useKnowledge(projectPath: string) {
  const [items, setItems] = useState<KnowledgeItem[]>([]);
  const [selectedDoc, setSelectedDoc] = useState<KnowledgeDocument | null>(null);
  const [stats, setStats] = useState<KnowledgeStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [docLoading, setDocLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const search = useCallback(async (query?: string, docType?: string) => {
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<KnowledgeItem[]>('list_knowledge', {
        projectPath,
        query: query || null,
        docType: docType || null,
        limit: 100,
      });
      setItems(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [projectPath]);

  const loadDoc = useCallback(async (docId: string) => {
    setDocLoading(true);
    try {
      const doc = await invoke<KnowledgeDocument>('get_knowledge_doc', {
        projectPath,
        docId,
      });
      setSelectedDoc(doc);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDocLoading(false);
    }
  }, [projectPath]);

  const loadStats = useCallback(async () => {
    try {
      const result = await invoke<KnowledgeStats>('get_knowledge_stats', {
        projectPath,
      });
      setStats(result);
    } catch {
      // stats are non-critical
    }
  }, [projectPath]);

  // Load initial data
  useEffect(() => {
    search();
    loadStats();
  }, [search, loadStats]);

  return {
    items,
    selectedDoc,
    stats,
    loading,
    docLoading,
    error,
    search,
    loadDoc,
    setSelectedDoc,
  };
}
