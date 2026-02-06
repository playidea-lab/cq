import { useCallback, useRef } from 'react';
import { Tldraw, Editor, createShapeId, TLComponents, JsonObject } from 'tldraw';
import 'tldraw/tldraw.css';
import type { CanvasData, CanvasNode, CanvasEdge } from '../types';

interface CanvasProps {
  data: CanvasData | null;
  onNodeSelect: (node: CanvasNode | null) => void;
  onEditorMount: (editor: Editor) => void;
  onNodePositionChange?: (nodeId: string, x: number, y: number) => void;
}

// Node type labels for tldraw frame names
const typeLabels: Record<string, string> = {
  document: 'DOC',
  config: 'CFG',
  session: 'SES',
  task: 'TSK',
  connection: 'CON',
};

// tldraw color type (used for both arrows and frames)
type TldrawColor = 'black' | 'blue' | 'green' | 'grey' | 'light-blue' | 'light-green' |
  'light-red' | 'light-violet' | 'orange' | 'red' | 'violet' | 'white' | 'yellow';

// Map node types to colors (matching Legend CSS)
// Config=violet, Document=blue, Task=yellow, Session=green, Connection=light-blue
const getNodeColor = (nodeType: string): TldrawColor => {
  switch (nodeType) {
    case 'config':
      return 'violet';     // purple/violet (#b55cb5)
    case 'document':
      return 'blue';        // blue (#5c8fbd)
    case 'task':
      return 'yellow';      // yellow (#b5a55c)
    case 'session':
      return 'green';       // green (#5cb58f)
    case 'connection':
      return 'light-blue';  // cyan/light-blue (#5ca5b5)
    default:
      return 'grey';
  }
};

// Map edge relations to colors
const getEdgeColor = (relation: string): TldrawColor => {
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
// Store edges for position updates
const edgeStore = new Map<string, CanvasEdge>();

// Constants for node dimensions
const NODE_WIDTH = 180;
const NODE_HEIGHT = 80;

// Helper function to update arrows connected to a node
function updateArrowsForNode(editor: Editor, nodeShapeId: string) {
  edgeStore.forEach((edge, edgeId) => {
    const sourceShapeId = createShapeId(edge.source);
    const targetShapeId = createShapeId(edge.target);

    // Check if this edge is connected to the moved node
    if (sourceShapeId === nodeShapeId || targetShapeId === nodeShapeId) {
      const arrowId = createShapeId(`edge-${edgeId}`);
      const arrow = editor.getShape(arrowId);
      if (!arrow) return;

      const sourceShape = editor.getShape(sourceShapeId);
      const targetShape = editor.getShape(targetShapeId);
      if (!sourceShape || !targetShape) return;

      // Calculate new arrow position
      const startX = sourceShape.x + NODE_WIDTH;
      const startY = sourceShape.y + NODE_HEIGHT / 2;
      const endX = targetShape.x;
      const endY = targetShape.y + NODE_HEIGHT / 2;

      // Update arrow
      editor.updateShape({
        id: arrowId,
        type: 'arrow',
        x: startX,
        y: startY,
        props: {
          start: { x: 0, y: 0 },
          end: { x: endX - startX, y: endY - startY },
        },
      });
    }
  });
}

export function Canvas({ onNodeSelect, onEditorMount, onNodePositionChange }: CanvasProps) {
  const editorRef = useRef<Editor | null>(null);
  const updateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleMount = useCallback((editor: Editor) => {
    editorRef.current = editor;
    onEditorMount(editor);

    // Listen for shape changes to update arrow positions
    editor.store.listen((entry) => {
      // Handle selection changes (immediate - no debounce for better UX)
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

        // Debounce arrow updates and position changes (300ms)
        // This prevents excessive re-renders during drag
        if (updateTimerRef.current) {
          clearTimeout(updateTimerRef.current);
        }

        updateTimerRef.current = setTimeout(() => {
          // Update arrows when frame shapes move
          Object.values(entry.changes.updated).forEach((record) => {
            const [_from, to] = record;
            if (to && to.typeName === 'shape' && to.type === 'frame') {
              updateArrowsForNode(editor, to.id);

              // Notify parent of position change
              if (onNodePositionChange && to.meta?.nodeId) {
                const nodeId = to.meta.nodeId as string;
                onNodePositionChange(nodeId, to.x, to.y);
              }
            }
          });
        }, 300);
      }
    });

    // Cleanup debounce timer on unmount
    return () => {
      if (updateTimerRef.current) {
        clearTimeout(updateTimerRef.current);
      }
    };
  }, [onEditorMount, onNodeSelect, onNodePositionChange]);

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
  edgeStore.clear();

  // Store nodes for later lookup
  data.nodes.forEach(node => {
    nodeStore.set(node.id, node);
  });

  // Store edges for position updates
  data.edges.forEach(edge => {
    edgeStore.set(edge.id, edge);
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
        name: `[${typeLabels[node.type] || '???'}] ${node.label}`,
        color: getNodeColor(node.type),
      },
      meta,
    };
  });

  editor.createShapes(nodeShapes);

  // Create arrows for edges
  const arrowShapes = data.edges.map((edge) => {
    const sourceNode = data.nodes.find(n => n.id === edge.source);
    const targetNode = data.nodes.find(n => n.id === edge.target);

    if (!sourceNode || !targetNode) return null;

    // Calculate initial arrow position
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
