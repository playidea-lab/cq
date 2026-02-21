/**
 * Generate /api/skills.jsonl and /api/tools.jsonl into the public/ directory.
 * Run: node scripts/generate-agent-files.mjs
 */

import { writeFileSync, mkdirSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const publicDir = join(__dirname, '../public/api')
mkdirSync(publicDir, { recursive: true })

// ── Skills ───────────────────────────────────────────────────────────────────

const skills = [
  { name: 'c4-plan',       trigger: ['/c4-plan', '계획', 'plan', '설계', '기획'],        description: 'Discovery → Design → Task creation. Full structured plan for a feature.', tier: 'all' },
  { name: 'c4-run',        trigger: ['/c4-run', '실행', 'run', 'ㄱㄱ'],                  description: 'Spawn workers for all pending tasks in parallel.', tier: 'all' },
  { name: 'c4-finish',     trigger: ['/c4-finish', '마무리', 'finish', '완료'],           description: 'Build → test → install → docs → commit. Post-implementation routine.', tier: 'all' },
  { name: 'c4-status',     trigger: ['/c4-status', '상태', 'status'],                    description: 'Visual task graph progress with queue summary and worker status.', tier: 'all' },
  { name: 'c4-quick',      trigger: ['/c4-quick', 'quick', '빠르게'],                    description: 'Create + assign one task immediately, skip planning phase.', tier: 'all' },
  { name: 'c4-polish',     trigger: ['/c4-polish', 'polish'],                            description: 'Build-test-review-fix loop until reviewer finds zero changes.', tier: 'all' },
  { name: 'c4-refine',     trigger: ['/c4-refine', 'refine'],                            description: 'Iterative review-fix loop using isolated worker context.', tier: 'all' },
  { name: 'c4-checkpoint', trigger: ['/c4-checkpoint'],                                  description: 'Phase gate review: approve, request-changes, replan, or redesign.', tier: 'all' },
  { name: 'c4-validate',   trigger: ['/c4-validate', '검증', 'validate'],                description: 'Run lint + unit tests. Severity-based: CRITICAL blocks commit.', tier: 'all' },
  { name: 'c4-add-task',   trigger: ['/c4-add-task', '태스크 추가', 'add task'],          description: 'Add new task interactively with DoD and scope guidance.', tier: 'all' },
  { name: 'c4-submit',     trigger: ['/c4-submit', '제출', 'submit'],                    description: 'Submit completed task with automated validation.', tier: 'all' },
  { name: 'c4-standby',    trigger: ['/c4-standby', '대기', 'standby', 'worker mode'],   description: 'Convert session into a persistent C5 Hub worker.', tier: 'full' },
  { name: 'c4-swarm',      trigger: ['/c4-swarm', 'swarm'],                              description: 'Spawn coordinator-led agent team for parallel C4 tasks.', tier: 'all' },
  { name: 'c4-stop',       trigger: ['/c4-stop', 'stop', '중단'],                        description: 'Stop execution, transition to HALTED state.', tier: 'all' },
  { name: 'c4-help',       trigger: ['/c4-help', 'help'],                                description: 'Quick reference for skills, agents, and MCP tools.', tier: 'all' },
]

const skillsJsonl = skills.map(s => JSON.stringify(s)).join('\n') + '\n'
writeFileSync(join(publicDir, 'skills.jsonl'), skillsJsonl)
console.log(`Generated api/skills.jsonl (${skills.length} skills)`)

// ── Tools ────────────────────────────────────────────────────────────────────

const tools = [
  // Status / Config
  { name: 'c4_status',           category: 'status',    tier: 'all',       description: 'Get project state, queue summary, and worker status.' },
  { name: 'c4_health',           category: 'status',    tier: 'all',       description: 'Check MCP server health.' },
  { name: 'c4_config_get',       category: 'config',    tier: 'all',       description: 'Get config value by key.' },
  { name: 'c4_config_set',       category: 'config',    tier: 'all',       description: 'Set config value by key.' },
  // Tasks
  { name: 'c4_add_todo',         category: 'task',      tier: 'all',       description: 'Add task to queue with DoD.' },
  { name: 'c4_get_task',         category: 'task',      tier: 'all',       description: 'Claim next available task (worker flow).' },
  { name: 'c4_submit',           category: 'task',      tier: 'all',       description: 'Submit completed task with commit SHA.' },
  { name: 'c4_claim',            category: 'task',      tier: 'all',       description: 'Claim task for direct mode (no worktree).' },
  { name: 'c4_report',           category: 'task',      tier: 'all',       description: 'Report task completion in direct mode.' },
  { name: 'c4_task_list',        category: 'task',      tier: 'all',       description: 'List all tasks with status.' },
  { name: 'c4_mark_blocked',     category: 'task',      tier: 'all',       description: 'Mark task as blocked with reason.' },
  // Files
  { name: 'c4_read_file',        category: 'file',      tier: 'all',       description: 'Read file with project path resolution.' },
  { name: 'c4_find_file',        category: 'file',      tier: 'all',       description: 'Find files by glob pattern.' },
  { name: 'c4_search_for_pattern', category: 'file',   tier: 'all',       description: 'Search file contents by regex.' },
  { name: 'c4_replace_content',  category: 'file',      tier: 'all',       description: 'Replace content in file.' },
  { name: 'c4_list_dir',         category: 'file',      tier: 'all',       description: 'List directory contents.' },
  // Knowledge
  { name: 'c4_knowledge_search', category: 'knowledge', tier: 'all',       description: 'Search knowledge base by query.' },
  { name: 'c4_knowledge_record', category: 'knowledge', tier: 'all',       description: 'Record insight, pattern, or experiment.' },
  { name: 'c4_knowledge_get',    category: 'knowledge', tier: 'all',       description: 'Get knowledge document by ID.' },
  // Secrets
  { name: 'c4_secret_set',       category: 'secret',    tier: 'all',       description: 'Store secret in AES-256-GCM store.' },
  { name: 'c4_secret_get',       category: 'secret',    tier: 'all',       description: 'Retrieve secret value.' },
  { name: 'c4_secret_list',      category: 'secret',    tier: 'all',       description: 'List secret keys (values masked).' },
  // LLM (connected+)
  { name: 'c4_llm_call',         category: 'llm',       tier: 'connected', description: 'Call LLM via unified gateway (Anthropic/OpenAI/Gemini/Ollama).' },
  { name: 'c4_llm_providers',    category: 'llm',       tier: 'connected', description: 'List available LLM providers and models.' },
  // Lighthouse
  { name: 'c4_lighthouse',       category: 'contract',  tier: 'all',       description: 'Tool contract registry (register/list/get/promote).' },
]

const toolsJsonl = tools.map(t => JSON.stringify(t)).join('\n') + '\n'
writeFileSync(join(publicDir, 'tools.jsonl'), toolsJsonl)
console.log(`Generated api/tools.jsonl (${tools.length} tools)`)
