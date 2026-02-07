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

export type ViewType = 'sessions' | 'dashboard' | 'config';

// --- Session types ---

export interface SessionMeta {
  id: string;
  slug: string;
  title: string | null;
  path: string;
  line_count: number;
  file_size: number;
  timestamp: number | null;
  git_branch: string | null;
}

export interface SessionPage {
  messages: SessionMessage[];
  total_lines: number;
  has_more: boolean;
}

export interface SessionMessage {
  msg_type: string;
  timestamp: string | null;
  uuid: string | null;
  content: ContentBlock[];
}

export interface ContentBlock {
  block_type: string;
  text: string | null;
  tool_name: string | null;
  tool_input: unknown | null;
}

export interface FileChange {
  path: string;
  backup_file: string | null;
  version: number | null;
  timestamp: string | null;
}

// --- Config types ---

export type ConfigCategory = 'global' | 'project' | 'persona' | 'c4' | 'memory';

export interface ConfigFileEntry {
  path: string;
  name: string;
  category: ConfigCategory;
  size: number;
  modified: number | null;
}

export interface ConfigFileContent {
  path: string;
  content: string;
  truncated: boolean;
}
