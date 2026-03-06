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

// --- Provider types ---

export type ProviderKind = 'claude_code' | 'codex_cli' | 'cursor' | 'gemini_cli';

export interface ProviderInfo {
  kind: ProviderKind;
  name: string;
  icon: string;
  session_count: number;
  data_path: string;
  is_global: boolean;
}

export interface TokenUsage {
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_creation_tokens: number;
  session_count: number;
}

// --- View types ---

export type ViewType = 'board' | 'docs' | 'knowledge' | 'messenger' | 'settings';

export type WorkspaceMode = 'messenger' | 'board' | 'docs' | 'knowledge' | 'settings';

// --- C1 Messaging types ---

export type ChannelType = 'general' | 'project' | 'knowledge' | 'session' | 'dm' | 'topic' | 'auto' | 'worker';
export type SenderType = 'human' | 'agent' | 'system';

export interface Channel {
  id: string;
  project_id: string;
  name: string;
  description: string;
  channel_type: ChannelType;
  created_at: string;
  updated_at: string;
}

export interface C1Message {
  id: string;
  channel_id: string;
  participant_id: string;
  content: string;
  thread_id: string | null;
  metadata: Record<string, unknown> | null;
  member_id?: string;
  agent_work_id?: string | null;
  created_at: string;
}

// --- C1 Member types ---

export type MemberType = 'user' | 'agent' | 'system';
export type MemberStatus = 'online' | 'working' | 'idle' | 'offline';

export interface C1Member {
  id: string;
  project_id: string;
  member_type: MemberType;
  external_id: string;
  display_name: string;
  avatar: string;
  status: MemberStatus;
  status_text: string;
  last_seen_at: string | null;
  created_at: string;
}

export interface C1MessagePage {
  messages: C1Message[];
  has_more: boolean;
  total: number;
}

export interface C1ChannelSummary {
  channel_id: string;
  unread_count: number;
  last_message_at: string | null;
  participant_count: number;
}

export interface C1Participant {
  id: string;
  channel_id: string;
  participant_id: string;
  last_read_at: string | null;
  joined_at: string;
}

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

export interface SearchHit {
  session_id: string;
  session_title: string | null;
  session_path: string;
  line_number: number;
  matched_text: string;
  context: string;
}

export interface FileChange {
  path: string;
  backup_file: string | null;
  version: number | null;
  timestamp: string | null;
}

// --- Dashboard Enhancement types (Phase 2) ---

export interface TaskEvent {
  task_id: string;
  title: string;
  status: string;
  task_type: string;
  updated_at: string | null;
  assigned_to: string | null;
}

export interface WorkerEvent {
  worker_id: string;
  task_id: string;
  action: string;
  timestamp: string | null;
}

export interface ValidationResult {
  name: string;
  passed: boolean;
  output: string;
}

// --- Git Graph types ---

export interface GitCommit {
  hash: string;
  shortHash: string;
  parents: string[];
  refs: string[];
  author: string;
  date: string;
  message: string;
}

// --- Session Analytics types ---

export interface ToolUsageStat {
  tool_name: string;
  count: number;
}

export interface SessionStats {
  total_messages: number;
  user_messages: number;
  assistant_messages: number;
  total_input_tokens: number;
  total_output_tokens: number;
  cache_read_tokens: number;
  estimated_cost_usd: number;
  tool_calls: ToolUsageStat[];
  duration_seconds: number;
  files_changed: number;
}

export interface DayStats {
  date: string; // "2026-02-08"
  session_count: number;
  total_tokens: number;
  estimated_cost: number;
}

// --- Cloud types (Phase 3) ---

export interface SyncResult {
  synced_count: number;
  errors: string[];
  last_synced: string;
}

export interface TeamProject {
  id: string;
  name: string;
  owner_email: string;
  task_count: number;
  done_count: number;
  status: string;
  last_updated: string | null;
}

// --- Cloud Phase 8.2 types ---

export interface PullResult {
  pulled_count: number;
  merged_count: number;
  conflict_count: number;
  errors: string[];
}

export interface SyncStatus {
  last_synced: string | null;
  pending_push: number;
  pending_pull: number;
  cloud_connected: boolean;
}

export interface RemoteCheckpoint {
  id: string;
  decision: string;
  notes: string | null;
  created_at: string;
}

export interface GrowthMetric {
  week: string;
  approval_rate: number;
  avg_score: number;
  tasks_completed: number;
}

export interface AgentTrace {
  agent_type: string;
  task_id: string | null;
  action: string;
  duration_ms: number | null;
  created_at: string;
}

// --- Knowledge Cloud types (Phase 8.3) ---

export interface KnowledgeDoc {
  doc_id: string;
  doc_type: 'experiment' | 'pattern' | 'insight' | 'hypothesis';
  title: string;
  domain: string;
  tags: string[];
  body: string;
  content_hash: string;
  version: number;
  created_at: string;
  updated_at: string;
}

// --- Auth types ---

export interface AuthUser {
  id: string;
  email: string;
  provider: string;
}

export interface AuthConfig {
  supabase_url: string | null;
  has_anon_key: boolean;
}

// --- Document types ---

export type DocType = 'persona' | 'skill' | 'spec' | 'config';

export interface DocumentMeta {
  name: string;
  doc_type: DocType;
  path: string;
  size: number;
  updated_at: string | null;
}

export interface DocumentContent {
  name: string;
  doc_type: DocType;
  content: string;
  path: string;
  updated_at: string | null;
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
