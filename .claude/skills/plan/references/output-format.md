# Phase 0 Output Format

ASCII templates for the C4 Plan Mode status display.

---

## Project Header

```
====================================================================
  {project_name} - {project_description}
  "{one-line project description}"
====================================================================
```

---

## Project Overview Section

```
--- Project Overview ---

  Project:    {project_id}
  Description: {from README.md}
  Domain:     {domain}
  License:    {license}

  Core Features:
  +--------------------------------------------------------------------+
  | * {feature1}    {description1}                                     |
  | * {feature2}    {description2}                                     |
  +--------------------------------------------------------------------+
```

Sources: README.md, LICENSE

---

## Current State Section

```
--- Current State ---

  Workflow:  INIT > DISCOVERY > DESIGN > PLAN > [EXECUTE] <> CHECKPOINT > COMPLETE
                                                     ^
                                                 current position

  Status:       {icon} {status} ({execution_mode})
  Supervisor:   {supervisor state}
  Workers:      {count} ({active/idle/disconnected details})

  +-------------------------------------------------------------+
  |  Progress:  {progress_bar}  {percentage}%                    |
  |             Done {done} / Total {total} tasks                |
  |                                                              |
  |  Done: {done}  In Progress: {ip}  Pending: {p}  Blocked: {b}|
  +-------------------------------------------------------------+
```

---

## Task Dependency Graph Section

```
--- Task Dependency Graph ---

[T-xxx] {series name} - {series description}
+-----------------------------------------------------------------------+
|                                                                       |
|  [done] T-xxx ----> [wip] T-xxx ----> [done] T-xxx ----> [wait] T-xxx|
|   {title}           {title}           {title}            {title}      |
|   (done)            (in progress)     (done)             (pending)    |
|                        |                                              |
|                        v                                              |
|                    [wait] T-xxx                                       |
|                     {title}                                           |
|                     (depends: T-xxx)                                  |
|                                                                       |
+-----------------------------------------------------------------------+

  Legend: [done]=completed  [wip]=in_progress  [wait]=pending  [block]=blocked
```

Rendering rules:
1. Show only dependency chains related to pending tasks
2. Start from root tasks (no dependencies)
3. Include recently completed tasks for context

---

## Specifications Section

```
--- Specifications ({count}) ---

+-----------------------------------------------------------------------+
| {feature_name}                                                        |
|    Domain: {domain}                                                   |
+-----------------------------------------------------------------------+
|                                                                       |
| Vision: {description}                                                 |
|                                                                       |
| Requirements ({count}, EARS patterns):                                |
|    +--------------+------------------------------------------------+  |
|    | ubiquitous   | {summarized ubiquitous requirements}           |  |
|    | event-driven | {summarized event-driven requirements}         |  |
|    | state-driven | {summarized state-driven requirements}         |  |
|    | optional     | {summarized optional requirements}             |  |
|    +--------------+------------------------------------------------+  |
|                                                                       |
| Status: {spec_status}                                                 |
+-----------------------------------------------------------------------+
```

---

## Designs Section

```
--- Designs ({count}) ---

+-----------------------------------------------------------------------+
| {feature_name}                                                        |
|    Domain: {domain}                                                   |
|    Selected: {selected_option}                                        |
+-----------------------------------------------------------------------+
|                                                                       |
| Architecture Options:                                                 |
|    [selected] {name}: {description}  [CHOSEN]                         |
|    [ ] {other}: {description}                                         |
|                                                                       |
| Components ({count}):                                                 |
|    {brief component list and relationships}                           |
|                                                                       |
| Key Decisions ({count}):                                              |
|    {decision summaries}                                               |
|                                                                       |
| NFR: {performance/memory/scalability summary}                         |
|                                                                       |
| Status: {design_status}                                               |
+-----------------------------------------------------------------------+
```

---

## Lighthouse Section

```
--- Lighthouse (Contract-First TDD) ---

  +-----------------------------------------------------------------------+
  | Stubs: {stubs}  Implemented: {impl}  Deprecated: {depr}              |
  |                                                                       |
  | Active Stubs:                                                         |
  |   [stub] {name1} -- {description1}     -> T-LH-{name1}-0            |
  |   [stub] {name2} -- {description2}     -> T-LH-{name2}-0            |
  |                                                                       |
  | Recently Implemented:                                                 |
  |   [impl] {name3} -- {description3}                                   |
  +-----------------------------------------------------------------------+
```

Display rules:
- Only show stub-status items in "Active Stubs"
- Implemented items: count summary only
- If no stubs: show "All tools implemented ({N})" one-liner

---

## Planning Documents Section

```
--- Planning Documents (docs/) ---

  docs/specs/ - Core spec documents
     {file list with brief descriptions}

  docs/{category}/ - {category description}
     {file list}
```

---

## Tech Stack Section

```
--- Tech Stack ---

  Language:    {language}
  Package:     {package_manager}
  Database:    {database}
  IDE:         {platforms}
  Validation:  {validation_tools}
```

Sources: pyproject.toml, package.json, Cargo.toml
