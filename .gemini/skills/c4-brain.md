# C4 Brain Skill

This skill enhances your ability to understand code and context. **Stop reading files blindly.** Use the specialized tools to "see" the code structure and "remember" the project history.

## Mandatory Rules (No Exceptions)
1. **Overview First**: Before calling `read_file` on any Python/TypeScript file you haven't seen in this session, you **MUST** call `c4_get_symbols_overview(relative_path="...")`. Use this to target your reading.
2. **Precision Navigation**: Use `c4_find_symbol(name_path_pattern="...")` to jump directly to definitions across the codebase.
3. **Memory Context**: Always call `c4_list_memories()` at the start of a deep investigation to see if there are relevant architectural constraints.

## Core Capabilities

### 1. Code Intelligence (LSP)
- **`c4_get_symbols_overview`**: Get the class/method outline of a file.
- **`c4_find_symbol`**: Find definitions (e.g., `C4Daemon.initialize`).
- **`c4_read_file`**: Read specific line ranges with line numbers.

### 2. Project Memory
- **`c4_read_memory(name="...")`**: Retrieve specific project knowledge (e.g., "coding-standards").
- **`c4_write_memory(name="...", content="...")`**: Save new lessons learned or decisions.

