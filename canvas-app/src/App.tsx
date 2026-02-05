import { useState, useCallback, useEffect, useRef } from 'react';
import { open } from '@tauri-apps/plugin-dialog';
import { Editor } from 'tldraw';
import { Canvas, renderCanvasData } from './components/Canvas';
import { DetailPanel } from './components/DetailPanel';
import { Toolbar } from './components/Toolbar';
import { Legend } from './components/Legend';
import { useScanner } from './hooks/useScanner';
import type { CanvasNode } from './types';

export default function App() {
  const [projectPath, setProjectPath] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<CanvasNode | null>(null);
  const editorRef = useRef<Editor | null>(null);
  const { loading, error, data, scanProject, loadCanvas, saveCanvas } = useScanner();

  // Handle editor mount
  const handleEditorMount = useCallback((editor: Editor) => {
    editorRef.current = editor;
  }, []);

  // Render data when it changes
  useEffect(() => {
    if (editorRef.current && data) {
      renderCanvasData(editorRef.current, data);
    }
  }, [data]);

  // Open folder dialog
  const handleOpenFolder = useCallback(async () => {
    try {
      const selected = await open({
        directory: true,
        multiple: false,
        title: 'Select C4 Project',
      });

      if (selected && typeof selected === 'string') {
        setProjectPath(selected);

        // Try to load saved canvas first
        const savedCanvas = await loadCanvas(selected);
        if (!savedCanvas) {
          // If no saved canvas, scan the project
          await scanProject(selected);
        }
      }
    } catch (err) {
      console.error('Failed to open folder:', err);
    }
  }, [loadCanvas, scanProject]);

  // Refresh scan
  const handleRefresh = useCallback(async () => {
    if (projectPath) {
      await scanProject(projectPath);
    }
  }, [projectPath, scanProject]);

  // Save canvas on changes (debounced)
  useEffect(() => {
    if (!projectPath || !data) return;

    const timeout = setTimeout(() => {
      saveCanvas(projectPath, data);
    }, 1000);

    return () => clearTimeout(timeout);
  }, [projectPath, data, saveCanvas]);

  // Handle node selection
  const handleNodeSelect = useCallback((node: CanvasNode | null) => {
    setSelectedNode(node);
  }, []);

  // Close detail panel
  const handleCloseDetail = useCallback(() => {
    setSelectedNode(null);
    if (editorRef.current) {
      editorRef.current.selectNone();
    }
  }, []);

  return (
    <div style={{ width: '100%', height: '100%', position: 'relative' }}>
      <Canvas
        data={data}
        onNodeSelect={handleNodeSelect}
        onEditorMount={handleEditorMount}
      />

      <Toolbar
        onRefresh={handleRefresh}
        onOpenFolder={handleOpenFolder}
        loading={loading}
        projectPath={projectPath || undefined}
      />

      <Legend />

      {selectedNode && (
        <DetailPanel
          node={selectedNode}
          onClose={handleCloseDetail}
        />
      )}

      {error && (
        <div style={{
          position: 'fixed',
          bottom: '80px',
          left: '16px',
          background: '#8b1a1a',
          color: '#fff',
          padding: '12px 16px',
          borderRadius: '8px',
          maxWidth: '400px',
          zIndex: 1000,
        }}>
          {error}
        </div>
      )}
    </div>
  );
}
