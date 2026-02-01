# C4 Brain Skill

This skill enhances your ability to understand code and context. **Stop reading files blindly.** Use the specialized tools to "see" the code structure and "remember" the project history.

## Core Capabilities

### 1. Code Intelligence (LSP)
Instead of `read_file` on large files, use:
- **`c4_get_symbols_overview`**: "Give me the outline of this file." (Classes, methods)
- **`c4_find_symbol`**: "Where is `UserAuth` defined?" or "Who calls `process_payment`?"
- **`c4_read_file`**: Use this for reading code with line numbers (better for diffs).

### 2. Project Memory
Access the collective knowledge of the project:
- **`c4_read_memory`**: Retrieve architecture decisions (ADR), style guides, or "lessons learned".
  - Examples: `c4_read_memory("style-guide")`, `c4_read_memory("gemini-rules")`.
- **`c4_list_memories`**: See what knowledge is available.

## Usage Rule
**ALWAYS** activate this skill when:
- You are debugging a complex issue.
- You are entering a new codebase area.
- You need to understand the "Why" behind a code block.
