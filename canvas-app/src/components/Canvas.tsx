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

// tldraw arrow color type
type ArrowColor = 'black' | 'blue' | 'green' | 'grey' | 'light-blue' | 'light-green' |
  'light-red' | 'light-violet' | 'orange' | 'red' | 'violet' | 'white' | 'yellow';

// Map edge relations to colors
const getEdgeColor = (relation: string): ArrowColor => {
  switch (relation) {
    case 'references':
      return 'blue';
    case 'creates':
      return 'green';
    case 'depends':
      return 'red';
    case 'applies':
      return 'orange';
    case 'mentions':
      return 'grey';
    default:
      return 'black';
  }
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

  // Create arrows for edges with calculated start/end points
  const NODE_WIDTH = 180;
  const NODE_HEIGHT = 80;

  const arrowShapes = data.edges.map((edge) => {
    const sourceNode = data.nodes.find(n => n.id === edge.source);
    const targetNode = data.nodes.find(n => n.id === edge.target);

    if (!sourceNode || !targetNode) return null;

    // Calculate edge start (right side of source) and end (left side of target)
    const startX = sourceNode.position.x + NODE_WIDTH;
    const startY = sourceNode.position.y + NODE_HEIGHT / 2;
    const endX = targetNode.position.x;
    const endY = targetNode.position.y + NODE_HEIGHT / 2;

    return {
      id: createShapeId(`edge-${edge.id}`),
      type: 'arrow' as const,
      x: startX,
      y: startY,
      props: {
        start: { x: 0, y: 0 },
        end: { x: endX - startX, y: endY - startY },
        color: getEdgeColor(edge.relation),
        size: 's' as const,
        arrowheadEnd: 'arrow' as const,
        arrowheadStart: 'none' as const,
      },
    };
  }).filter(Boolean);

  // Create arrows after nodes
  if (arrowShapes.length > 0) {
    editor.createShapes(arrowShapes as NonNullable<typeof arrowShapes[number]>[]);
  }

  // Zoom to fit all content
  setTimeout(() => {
    editor.zoomToFit();
  }, 100);
}
