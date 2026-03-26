---
name: review
description: |
  Comprehensive 3-pass academic paper review with detailed math verification,
  formal tone rules, and 6-axis evaluation (quality, novelty, technical soundness,
  experimental validation, discussion, presentation). Generates formal review
  documents with C2 lifecycle integration and persona learning from human edits.
  Use for in-depth reviews requiring equation verification or detailed technical
  analysis. Triggers: "리뷰 작성", "상세 논문 리뷰", "6축 리뷰", "수식 검증 리뷰",
  "academic review", "/review". For quick reviews, use /c2-paper-review.
allowed-tools: Read, Glob, Grep, Write, mcp__cq__*
---

# review: Academic Paper Review (c2 Lifecycle)

> **Command**: `/review <pdf_path> [--project <name>] [--journal <name>]`

## Phase 1: Setup

1. **Parse args**: `pdf_path` (required), `--project` (default: filename), `--journal`
2. **Load config**: `.c2/profile.yaml` (6-axis framework), `.c2/guides/review.md`, `.c2/persona.md`
   - Or via MCP: `c4_profile_load(profile_path: ".c2/profile.yaml")`
3. **Project setup**: Create `projects/{name}/` with `discover/read/write/review/artifacts/` dirs
4. **Workspace**: Load or create via `c4_workspace_create/load/save`
5. **Knowledge 조회**: `c4_knowledge_search(query="{title} {domain}")` for past review patterns

## Phase 2: Pass 1 — Structure Analysis

Read PDF, create section map, summarize core contribution, record first impressions.
Save to `projects/{name}/read/{source_id}_note.md`.
**Checkpoint**: Ask user for focus areas before proceeding.

## Phase 3: Pass 2 — 6-Axis Detailed Analysis

Evaluate against 6 dimensions (weights: subject=1.0, novelty=1.0, technical=1.5, experimental=1.5, discussion=1.0, presentation=0.8). Score each 1-10.
See `references/analysis-phases.md` for detailed checklists and math verification.
Save to `projects/{name}/review/review_workspace.md`.
**Checkpoint**: Ask user for dimension-level feedback.

## Phase 4: Pass 3 — Overall Assessment

1. Compute weighted average → recommendation (1-3 Reject, 4-5 Major Rev, 6-7 Minor Rev, 8-10 Accept)
2. Classify Major vs Minor comments
3. Build Claim-Evidence mapping
**Checkpoint**: Confirm score and recommendation with user.

## Phase 5: Draft Generation

Generate formal review following tone rules. See `references/review-template.md` for structure.

### CRITICAL: Tone Rules

1. **Question form**: "확인이 필요합니다" (NOT "오류가 있습니다")
2. **Humility**: "제가 잘못이해한 부분이 있다면 알려주시면 감사하겠습니다"
3. **Editor softness**: "오류가 아닌가 생각됩니다"
4. **Evidence separation**: Detailed verification in workspace, key findings only in review

### Output Files

- `review/.draft.md` — AI original (for persona learning)
- `review/리뷰의견.md` — User edits this
- Tell user: "수정이 완료되면 알려주세요."

## Phase 5.5: Knowledge Recording

```
c4_knowledge_record(doc_type: "insight", title: "Review: {title}", tags: ["review", "{venue}"])
```

## Phase 6: Persona Learning

After user edits `리뷰의견.md`:

```
c4_persona_learn(draft_path: ".draft.md", final_path: "리뷰의견.md", profile_path: ".c2/profile.yaml", auto_apply: false)
```

Show discovered patterns → apply with user approval.

## Usage Examples

```
/review paper.pdf
/review paper.pdf --project 25-TIE-6582 --journal "IEEE TIE"
```
