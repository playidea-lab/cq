# C4 Multi-Provider Strategy Plan

## Phase 1: Standardization ("Single Source of Truth")
**Goal:** Remove provider-specific dependency on `.claude/rules` and establish provider-neutral `.c4/standards`.

1.  **Directory Structure Overhaul**
    - Create `.c4/standards`.
    - Move `.claude/rules/*.md` to `.c4/standards`/.
2.  **Backward Compatibility (Symlink/Copy)**
    - Modify `c4 init` and `install.sh`:
        - Ensure `.claude/rules` exists.
        - Create **symbolic links** (or copy on Windows) from `.c4/standards/*.md` to `.claude/rules`/.
        - **Reasoning:** Claude Code CLI still hard-codes `.claude/rules` for context.
3.  **Validation**: Verify `c4 init` keeps Claude Code context awareness intact.

## Phase 2: Abstraction ("Provider Strategy Pattern")
**Goal:** Refactor `LiteLLMBackend` to use distinct parameter strategies based on the model name (Gemini, GPT, Claude).

1.  **Define `ProviderStrategy` Interface** (`c4/supervisor/strategies.py`)
    - `get_request_params(prompt, system_message)`: Return model-specific parameters.
    - `handle_response(response)`: Parse model-specific response formats.
2.  **Implement Strategies**
    - **`GeminiStrategy`**:
        - `safety_settings`: `BLOCK_NONE` (Essential for code generation).
        - `response_format`: `{"type": "json_object"}` (Native structured output).
        - `role_mapping`: Map `system` to `developer` or `system_instruction` as supported by LiteLLM/Gemini.
    - **`ClaudeStrategy`** (Default): Keep existing logic (XML optimizations).
    - **`OpenAIStrategy`**: Apply `response_format={"type": "json_object"}`.
3.  **Refactor `LiteLLMBackend`**
    - Inject appropriate Strategy instance in constructor based on `model` string.

## Phase 3: Injection ("Dynamic Brain")
**Goal:** Dynamically inject `.c4/standards` content into the System Instruction when the Supervisor runs.

1.  **Implement `ContextLoader`**
    - Utility to read and merge `.c4/standards/*.md` into a single context string.
2.  **Enhance System Prompt**
    - **Before:** "You are a code review supervisor..."
    - **After:** `ContextLoader.load()` + "\n\nYou are a code review supervisor..."
3.  **Gemini System Instruction Mapping**
    - In `GeminiStrategy`, map this enhanced prompt to the `system_instruction` parameter (or equivalent LiteLLM field).

## Phase 4: Validation
**Goal:** Verify code reviews succeed with actual Gemini models.

1.  **`debug_gemini_review.py`**: Simulate a real C4 Supervisor review.
2.  **Safety Verification**: Ensure "dangerous" code (e.g., `os.system`) is reviewed, not blocked.
3.  **JSON Verification**: Ensure Gemini returns valid JSON strictly.
