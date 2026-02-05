// Canvas data model types

export type NodeType = 'document' | 'config' | 'session' | 'task' | 'connection';

export interface CanvasNode {
  id: string;
  type: NodeType;
  label: string;
  path?: string;
  metadata: Record<string, unknown>;
  position: { x: number; y: number };
  timestamp?: number; // Unix timestamp for time-based layout
}

export interface CanvasEdge {
  id: string;
  source: string; // node id
  target: string; // node id
  relation: 'references' | 'creates' | 'depends' | 'applies' | 'mentions';
}

export interface CanvasData {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  viewport: {
    x: number;
    y: number;
    zoom: number;
  };
}

export interface ScanResult {
  success: boolean;
  data?: CanvasData;
  error?: string;
}
