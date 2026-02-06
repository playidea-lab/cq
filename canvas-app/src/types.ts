// Canvas data model types (legacy, kept for scanner compatibility)

export type NodeType = 'document' | 'config' | 'session' | 'task' | 'connection';

export interface CanvasNode {
  id: string;
  type: NodeType;
  label: string;
  path?: string;
  metadata: Record<string, unknown>;
  position: { x: number; y: number };
  timestamp?: number;
}

export interface CanvasEdge {
  id: string;
  source: string;
  target: string;
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

// --- Dashboard types ---

export type ProjectStatus =
  | 'INIT'
  | 'DISCOVERY'
  | 'DESIGN'
  | 'PLAN'
  | 'EXECUTE'
  | 'CHECKPOINT'
  | 'COMPLETE'
  | 'HALTED'
  | 'ERROR';

export type TaskStatus = 'pending' | 'in_progress' | 'done' | 'blocked';

export type TaskType = 'IMPLEMENTATION' | 'REVIEW' | 'CHECKPOINT';

export interface ProjectState {
  status: ProjectStatus;
  project_id: string;
  workers: WorkerInfo[];
  progress: TaskProgress;
}

export interface WorkerInfo {
  id: string;
  status: 'idle' | 'busy';
  current_task: string | null;
  last_seen: number | null;
}

export interface TaskProgress {
  total: number;
  done: number;
  in_progress: number;
  pending: number;
  blocked: number;
}

export interface TaskItem {
  id: string;
  title: string;
  status: TaskStatus;
  task_type: TaskType;
  dependencies: string[];
  assigned_to: string | null;
  domain: string | null;
  priority: number;
}

export interface TaskDetail extends TaskItem {
  dod: string;
  scope: string | null;
  branch: string | null;
  commit_sha: string | null;
  version: number;
  parent_id: string | null;
  review_decision: string | null;
  validations: string[];
}

// --- View types ---

export type ViewType = 'dashboard' | 'registry' | 'timeline';
