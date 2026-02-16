---
description: |
  Conduct academic paper reviews using the C2 document lifecycle system with a
  6-axis evaluation framework. Performs 3-pass review (structure, detailed analysis,
  synthesis), generates bilingual review documents, and learns from user edits via
  persona learning. Uses MCP tools for workspace management and profile updates.
  Trigger: "review this paper", "c2 review", "/c2-review".
---

# c2-review: Academic Paper Review (C2 Lifecycle)

> **Command**: `/c2-review <pdf_path> [--project <name>] [--journal <name>]`
>
> Full 3-pass academic paper review using c2 document lifecycle. Claude reads PDF directly, applies 6-axis framework, generates bilingual reviews, and learns from user feedback.

## Key Features vs /review
- **Settings**: `.c2/profile.yaml` (not `.auto_review/profile.yaml`)
- **Output**: `projects/{name}/review/` (not `review/`)
- **Workspace**: `c2_workspace.md` auto-managed via MCP tools
- **Analysis**: Claude reads PDF directly with 6-axis framework
- **Learning**: Draft vs final diff analysis via `c4_persona_learn`

---

## Phase 1: Setup

### Parse Arguments
```
$ARGUMENTS → parse:
  pdf_path: (required) path to PDF file
  --project: project name (default: derive from filename)
  --journal: journal name for context (e.g., "IEEE TIE", "NeurIPS")
```

### Load Configuration
Read ALL configuration files (warn if missing, continue):
1. `.c2/profile.yaml` — 6-axis framework, tone rules, review points, style guide
2. `.c2/guides/review.md` — Review methodology (3-Pass, section structure, tone rules)
3. `.c2/persona.md` — Reviewer persona details

Alternative: `c4_profile_load(profile_path: ".c2/profile.yaml")`

### Project Setup
```
project_name = args.get("project") or Path(pdf_path).stem
project_dir = Path(f"projects/{project_name}")

Create directories:
  projects/{project_name}/{discover, read, write, review, artifacts}/

Copy/symlink PDF to discover/
```

### Workspace Initialization
Check if `projects/{project_name}/c2_workspace.md` exists:
- **Exists**: `c4_workspace_load(project_dir: "projects/{project_name}")`
- **Not exists**:
  ```
  c4_workspace_create(name: project_name, project_type: "academic_paper", goal: "{journal or ''} paper review")
  c4_workspace_save(project_dir: "projects/{project_name}", state: <returned state>)
  ```

---

## Phase 2: Pass 1 — Structure Analysis

### Goal
Map paper structure, contributions, and initial impressions.

### Instructions
1. Read PDF using Read tool
2. Create section map: title, page range, 1-line summary per section
3. Summarize core contribution (author's claim)
4. Record first impressions: strengths, concerns, focus areas
5. List assumptions (explicit + implicit)

### Output
Save to `projects/{project_name}/read/{source_id}_note.md`:
```markdown
# Reading Note: {paper_title}

## Pass 1: Structure Analysis
- **Title**: ...
- **Authors**: ...
- **Journal/Venue**: ...

### Section Map
| Section | Pages | Summary |
|---------|-------|---------|
| ... | ... | ... |

### Core Contribution (author's claim)
...

### First Impressions
- Strengths: ...
- Concerns: ...
- Focus areas: ...

### Assumptions (explicit + implicit)
1. ...
```

### Workspace Update
Update `c2_workspace.md`:
- Add paper to Discover table (status: 완료)
- Add reading note to Read table
- Update last_session

### Interactive Checkpoint
Ask user: **"Pass 1 구조 분석을 완료했습니다. 집중적으로 볼 부분이 있나요?"**

Wait for response before proceeding.

---

## Phase 3: Pass 2 — 6-Axis Detailed Analysis

### Goal
Evaluate paper against 6 dimensions from `profile.yaml`.

### 6-Axis Framework
Apply from `.c2/profile.yaml → review_framework.dimensions`:
1. **Quality of Subject** (weight: 1.0) — motivation, significance, relevance
2. **Novelty / Originality** (weight: 1.0) — new approach, contribution scope
3. **Technical Soundness** (weight: 1.5) — assumptions, derivations, edge cases
4. **Experimental Validation** (weight: 1.5) — baselines, conditions, statistics
5. **Discussion & Completeness** (weight: 1.0) — interpretation, limitations, future
6. **Presentation Quality** (weight: 0.8) — flow, figures, formatting

For each dimension:
- Evaluate against checklist items
- Assign score (1-10)
- Note specific issues with references (equation #, figure #, page #)

### Math Verification (if applicable)
For papers with mathematical derivations:
- Verify key equations step-by-step
- Check limiting cases
- Create `artifacts/` scripts for numerical verification if needed
- Save detailed verification to `review/math_verification.md`

### Output
Save to `projects/{project_name}/review/review_workspace.md`:
```markdown
# Review Workspace: {paper_title}

## 6-Axis Evaluation

### 1. Quality of Subject [score: X/10]
- [ ] checklist item → assessment
...

### 2. Novelty [score: X/10]
...

(continue for all 6 dimensions)

## Detailed Evidence
(MC simulations, numerical comparisons, derivation checks — all detailed evidence here)
```

### Workspace Update
Update `c2_workspace.md` changelog.

### Interactive Checkpoint
Ask user: **"6축 상세 분석을 완료했습니다. 차원별 분석에 의견이 있나요?"**

Wait for response before proceeding.

---

## Phase 4: Pass 3 — Overall Assessment

### Goal
Synthesize dimension scores into overall judgment.

### Instructions
1. **Compute weighted average**:
   ```
   score = Σ(dimension_score × weight) / Σ(weight)
   weights: subject=1.0, novelty=1.0, technical=1.5, experimental=1.5, discussion=1.0, presentation=0.8
   ```

2. **Determine recommendation**:
   - 1-3: Reject
   - 4-5: Major Revision
   - 6-7: Minor Revision
   - 8-10: Accept

3. **Classify issues**:
   - **Major Comments**: Issues requiring significant revision (methodology flaws, missing evidence, incorrect derivations)
   - **Minor Comments**: Smaller improvements (presentation, missing details, formatting)

4. **Build Claim-Evidence mapping**: For each major claim, map to evidence and assess strength

### Workspace Update
Update `c2_workspace.md`:
- Review table entry
- Claim-Evidence table
- Changelog entry with decision

### Interactive Checkpoint
Ask user: **"종합 점수 {score}/10, 판정: {recommendation}. 판정에 대해 의견이 있나요?"**

Wait for response before proceeding.

---

## Phase 5: Finalize — Draft Generation

### Goal
Generate formal review document following c2 tone rules.

### CRITICAL: Tone Rules
Apply STRICTLY from profile + guide:
1. **Question form for technical issues**: "확인이 필요합니다", "확인 바랍니다" (NOT "오류가 있습니다")
2. **Humility**: "제가 잘못이해한 부분이 있다면 알려주시면 감사하겠습니다"
3. **Editor softness**: "오류가 아닌가 생각됩니다" (NOT "오류가 있습니다")
4. **Evidence separation**: Detailed verification stays in `review_workspace.md`, only key findings in review

### Review Structure

**Korean**:
```
[인사말 — 리뷰어 역할 맡게되어 영광]
[논문 요약 + 긍정 마무리]
[전환구]

A. 주요 의견 (Major Comments)
  1. (optional) 동기/contribution 메타 코멘트
  2~. 기술 이슈 (질문형으로)

B. 보조 의견 (Minor Comments)
  (번호 매김, 간결하게 1-2줄)

C. 그 밖에 (선택적 — Regular Paper면 유지, Letter면 생략 가능)

[개인 의견 + 마무리]
감사합니다.

---
에디터에게
[판정 근거 + 추천]
감사합니다.
```

**English**:
```
[Opening — honor to serve as reviewer]
[Paper summary + positive note]
[Transition]

A. Major Comments
B. Minor Comments
C. Additional Remarks (optional)

[Closing]
Thank you.

---
To the Editor:
[Assessment + recommendation]
Thank you.
```

### Output Files
1. **`review/.draft.md`** — AI-generated original (for persona learning)
2. **`review/리뷰의견.md`** — Initial version (same as draft initially)

Both files contain full Korean + English review.

### Interactive Checkpoint
Tell user:
> "리뷰 초안을 저장했습니다."
> - `projects/{name}/review/.draft.md` (AI 원본 — 학습용)
> - `projects/{name}/review/리뷰의견.md` (수정용)
>
> "수정이 완료되면 알려주세요. 수정 패턴을 분석하여 프로필을 업데이트합니다."

**IMPORTANT**: User ALWAYS reviews and edits the draft. Do not skip this step.

---

## Phase 6: Persona Learning

### Trigger
User indicates they have finished editing `리뷰의견.md`.

### Instructions
Run persona learning via MCP tool:
```
c4_persona_learn(
  draft_path: "projects/{name}/review/.draft.md",
  final_path: "projects/{name}/review/리뷰의견.md",
  profile_path: ".c2/profile.yaml",
  auto_apply: false
)
→ returns {summary, new_patterns, tone_updates, structure_updates}
```

### Present Results
Show user:
> "발견된 패턴:"
> - [tone] User softened tone: ...
> - [structure] User shortened text: ...
>
> "이 패턴을 프로필에 반영할까요?"

### Apply (with user approval)
```
c4_persona_learn(
  draft_path: "projects/{name}/review/.draft.md",
  final_path: "projects/{name}/review/리뷰의견.md",
  profile_path: ".c2/profile.yaml",
  auto_apply: true
)
```

### Final Workspace Update
Update `c2_workspace.md`:
- Review reflection_status → 반영완료
- Changelog entry for persona learning

---

## Error Handling

- **PDF not found**: "PDF 파일을 찾을 수 없습니다: {path}"
- **Missing profile**: "`.c2/profile.yaml`이 없습니다. 기본 설정으로 진행합니다."
- **Missing guide**: "`.c2/guides/review.md`가 없습니다. 기본 방법론으로 진행합니다."
- **Parse errors**: If workspace parsing fails, create a fresh workspace

---

## Usage Examples

```bash
/c2-review paper.pdf
/c2-review paper.pdf --project 25-TIE-6582 --journal "IEEE TIE"
/c2-review ~/Downloads/submission.pdf --project new-review
```
