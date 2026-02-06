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
  const [errorVisible, setErrorVisible] = useState<boolean>(true);
  const editorRef = useRef<Editor | null>(null);
  const { loading, error, data, scanProject, loadCanvas, saveCanvas, setData } = useScanner();

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

  // Check if running inside Tauri
  const isTauri = Boolean(
    typeof window !== 'undefined' && (window as any).__TAURI_INTERNALS__
  );

  // Open folder dialog
  const handleOpenFolder = useCallback(async () => {
    if (!isTauri) {
      // Fallback: prompt for path when not in Tauri webview
      const path = window.prompt('Enter project path (Tauri dialog not available in browser):');
      if (path) {
        setProjectPath(path);
        const savedCanvas = await loadCanvas(path);
        if (!savedCanvas) {
          await scanProject(path);
        }
      }
      return;
    }

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
  }, [isTauri, loadCanvas, scanProject]);

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

  // Handle node position change from drag
  const handleNodePositionChange = useCallback((nodeId: string, x: number, y: number) => {
    if (!data) return;

    // Update the node's position in data
    const updatedNodes = data.nodes.map(node =>
      node.id === nodeId
        ? { ...node, position: { x, y } }
        : node
    );

    setData({
      ...data,
      nodes: updatedNodes,
    });
  }, [data, setData]);

  // Close detail panel
  const handleCloseDetail = useCallback(() => {
    setSelectedNode(null);
    if (editorRef.current) {
      editorRef.current.selectNone();
    }
  }, []);

  // Auto-dismiss error toast after 5 seconds
  useEffect(() => {
    if (error) {
      setErrorVisible(true);
      const timer = setTimeout(() => {
        setErrorVisible(false);
      }, 5000);

      return () => clearTimeout(timer);
    }
  }, [error]);

  // Handle manual error close
  const handleCloseError = useCallback(() => {
    setErrorVisible(false);
  }, []);

  return (
    <div className="app-container">
      <Canvas
        data={data}
        onNodeSelect={handleNodeSelect}
        onEditorMount={handleEditorMount}
        onNodePositionChange={handleNodePositionChange}
      />

      <Toolbar
        onRefresh={handleRefresh}
        onOpenFolder={handleOpenFolder}
        loading={loading}
        projectPath={projectPath || undefined}
      />

      <Legend />

      {/* Empty state: No project selected */}
      {!projectPath && !loading && (
        <div className="empty-state">
          <h2 className="empty-state__title">
            Welcome to C4 Canvas
          </h2>
          <p className="empty-state__description">
            Select a project folder to visualize your C4 project structure, tasks, and dependencies.
          </p>
          <button
            className="btn btn--primary"
            onClick={handleOpenFolder}
          >
            Open Project Folder
          </button>
        </div>
      )}

      {/* Empty state: No nodes in scanned project */}
      {projectPath && data && data.nodes.length === 0 && !loading && !error && (
        <div className="empty-state">
          <h3 className="empty-state__title empty-state__title--sm">
            No Nodes Found
          </h3>
          <p className="empty-state__description empty-state__description--compact">
            The selected project doesn't contain any C4 data yet. Try refreshing or select a different folder.
          </p>
          <div className="empty-state__actions">
            <button
              className="btn btn--primary btn--sm"
              onClick={handleRefresh}
            >
              Refresh
            </button>
            <button
              className="btn btn--secondary btn--sm"
              onClick={handleOpenFolder}
            >
              Change Folder
            </button>
          </div>
        </div>
      )}

      {selectedNode && (
        <DetailPanel
          node={selectedNode}
          onClose={handleCloseDetail}
        />
      )}

      {error && errorVisible && (
        <div className="error-toast" role="alert" aria-live="assertive">
          <span className="error-toast__message">{error}</span>
          <button
            className="error-toast__close"
            onClick={handleCloseError}
            aria-label="Close error notification"
          >
            ×
          </button>
        </div>
      )}
    </div>
  );
}
