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
    <div style={{ width: '100%', height: '100%', position: 'relative' }}>
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
        <div style={{
          position: 'absolute',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          textAlign: 'center',
          zIndex: 100,
          padding: '32px',
          background: 'rgba(255, 255, 255, 0.95)',
          borderRadius: '12px',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.1)',
          maxWidth: '400px',
        }}>
          <h2 style={{ margin: '0 0 16px', fontSize: '24px', color: '#333' }}>
            Welcome to C4 Canvas
          </h2>
          <p style={{ margin: '0 0 24px', color: '#666', lineHeight: '1.5' }}>
            Select a project folder to visualize your C4 project structure, tasks, and dependencies.
          </p>
          <button
            onClick={handleOpenFolder}
            style={{
              padding: '12px 24px',
              fontSize: '16px',
              background: '#0066cc',
              color: '#fff',
              border: 'none',
              borderRadius: '8px',
              cursor: 'pointer',
              fontWeight: 500,
              transition: 'background 0.2s',
            }}
            onMouseOver={(e) => e.currentTarget.style.background = '#0052a3'}
            onMouseOut={(e) => e.currentTarget.style.background = '#0066cc'}
          >
            📁 Open Project Folder
          </button>
        </div>
      )}

      {/* Empty state: No nodes in scanned project */}
      {projectPath && data && data.nodes.length === 0 && !loading && !error && (
        <div style={{
          position: 'absolute',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          textAlign: 'center',
          zIndex: 100,
          padding: '32px',
          background: 'rgba(255, 255, 255, 0.95)',
          borderRadius: '12px',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.1)',
          maxWidth: '400px',
        }}>
          <h3 style={{ margin: '0 0 12px', fontSize: '20px', color: '#333' }}>
            No Nodes Found
          </h3>
          <p style={{ margin: '0 0 16px', color: '#666', lineHeight: '1.5' }}>
            The selected project doesn't contain any C4 data yet. Try refreshing or select a different folder.
          </p>
          <div style={{ display: 'flex', gap: '12px', justifyContent: 'center' }}>
            <button
              onClick={handleRefresh}
              style={{
                padding: '10px 20px',
                fontSize: '14px',
                background: '#0066cc',
                color: '#fff',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                fontWeight: 500,
              }}
            >
              🔄 Refresh
            </button>
            <button
              onClick={handleOpenFolder}
              style={{
                padding: '10px 20px',
                fontSize: '14px',
                background: '#666',
                color: '#fff',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                fontWeight: 500,
              }}
            >
              📁 Change Folder
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
          display: 'flex',
          alignItems: 'center',
          gap: '12px',
        }}>
          <span style={{ flex: 1 }}>{error}</span>
          <button
            onClick={handleCloseError}
            style={{
              background: 'transparent',
              border: 'none',
              color: '#fff',
              cursor: 'pointer',
              fontSize: '18px',
              padding: '0',
              lineHeight: '1',
            }}
            aria-label="Close error notification"
          >
            ×
          </button>
        </div>
      )}
    </div>
  );
}
