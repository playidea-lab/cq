import { useCallback } from 'react';
import { Tldraw, Editor, createShapeId, TLComponents, JsonObject } from 'tldraw';
import 'tldraw/tldraw.css';
import type { CanvasData, CanvasNode } from '../types';

interface CanvasProps {
  data: CanvasData | null;
  onNodeSelect: (node: CanvasNode | null) => void;
  onEditorMount: (editor: Editor) => void;
}

// Map node types to icons (emojis for simplicity)
const typeIcons: Record<string, string> = {
  document: '📄',
  config: '⚙️',
  session: '💬',
  task: '✅',
  connection: '🔗',
};

// Hide default tldraw UI components we don't need
const components: TLComponents = {
  DebugPanel: null,
  DebugMenu: null,
  SharePanel: null,
  MenuPanel: null,
  TopPanel: null,
  HelpMenu: null,
};

// Store nodes separately for lookup (tldraw meta only accepts JsonObject)
const nodeStore = new Map<string, CanvasNode>();

export function Canvas({ onNodeSelect, onEditorMount }: CanvasProps) {
  const handleMount = useCallback((editor: Editor) => {
    onEditorMount(editor);

    // Listen for selection changes
    editor.store.listen((entry) => {
      if (entry.changes.updated) {
        const selectedIds = editor.getSelectedShapeIds();
        if (selectedIds.length === 1) {
          const shapeId = selectedIds[0];
          const shape = editor.getShape(shapeId);
          if (shape && shape.meta?.nodeId) {
            const node = nodeStore.get(shape.meta.nodeId as string);
            onNodeSelect(node || null);
          } else {
            onNodeSelect(null);
          }
        } else {
          onNodeSelect(null);
        }
      }
    });
  }, [onEditorMount, onNodeSelect]);

  return (
    <Tldraw
      onMount={handleMount}
      components={components}
      inferDarkMode
    />
  );
}

// Helper function to render canvas data into tldraw editor
export function renderCanvasData(editor: Editor, data: CanvasData) {
  // Clear stores
  nodeStore.clear();

  // Store nodes for later lookup
  data.nodes.forEach(node => {
    nodeStore.set(node.id, node);
  });

  // Clear existing shapes
  const existingShapes = editor.getCurrentPageShapes();
  if (existingShapes.length > 0) {
    editor.deleteShapes(existingShapes.map(s => s.id));
  }

  // Create shapes for nodes using frame shapes
  const nodeShapes = data.nodes.map((node) => {
    const shapeId = createShapeId(node.id);
    const meta: JsonObject = {
      nodeId: node.id,
      nodeType: node.type,
    };

    return {
      id: shapeId,
      type: 'frame' as const,
      x: node.position.x,
      y: node.position.y,
      props: {
        w: 180,
        h: 80,
        name: `${typeIcons[node.type] || '📦'} ${node.label}`,
      },
      meta,
    };
  });

  editor.createShapes(nodeShapes);

  // TODO: Add edge rendering in v2 with proper tldraw v4 binding API
  // For MVP, edges are stored but not visually rendered

  // Zoom to fit all content
  setTimeout(() => {
    editor.zoomToFit();
  }, 100);
}
