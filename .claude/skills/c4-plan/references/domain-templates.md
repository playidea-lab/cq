# Domain Templates

Domain-specific interview questions, detection rules, and architecture options for C4 Plan Mode.

---

## Domain Detection Rules

| Domain | Detection Condition |
|--------|-------------------|
| `web-frontend` | `package.json` + (react\|vue\|angular\|svelte) |
| `web-backend` | `package.json` + (express\|fastify\|nest) or `pyproject.toml` + (fastapi\|flask\|django) |
| `ml-dl` | `pyproject.toml` + (torch\|tensorflow\|jax) or `*.ipynb` exists |
| `mobile-app` | `pubspec.yaml` (Flutter) or `react-native` |
| `infra` | `*.tf` (Terraform) or `docker-compose.yml` |
| `library` | `setup.py` or `pyproject.toml` (build-system section) |

---

## Web Frontend (`web-frontend`)

### Requirements Interview

```python
AskUserQuestion(questions=[
    {
        "question": "Which UI features are needed?",
        "header": "UI Features",
        "options": [
            {"label": "Auth/Login", "description": "Login, signup, session"},
            {"label": "Dashboard", "description": "Charts, stats, monitoring"},
            {"label": "Forms/Input", "description": "Data entry and validation"},
            {"label": "List/Table", "description": "Data list display"}
        ],
        "multiSelect": True
    },
    {
        "question": "Which interactions are needed?",
        "header": "Interaction",
        "options": [
            {"label": "None", "description": "Basic click/input only"},
            {"label": "Drag and drop", "description": "Item reordering"},
            {"label": "Real-time updates", "description": "WebSocket, SSE"},
            {"label": "Gestures", "description": "Swipe, pinch, etc."}
        ],
        "multiSelect": True
    }
])
```

### Architecture Options

```python
# State management
AskUserQuestion(questions=[
    {
        "question": "Select state management pattern",
        "header": "State",
        "options": [
            {"label": "Context API (recommended)", "description": "React built-in, small projects"},
            {"label": "Redux", "description": "Large scale, complex state"},
            {"label": "Zustand", "description": "Lightweight, simple API"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select folder structure",
        "header": "Structure",
        "options": [
            {"label": "Feature-based (recommended)", "description": "Group by feature"},
            {"label": "Type-based", "description": "Components/hooks/utils separation"},
            {"label": "Atomic Design", "description": "atoms/molecules/organisms"}
        ],
        "multiSelect": False
    }
])
```

---

## Web Backend (`web-backend`)

### Requirements Interview

```python
AskUserQuestion(questions=[
    {
        "question": "Which API features are needed?",
        "header": "API",
        "options": [
            {"label": "REST CRUD", "description": "Basic resource management"},
            {"label": "Auth API", "description": "JWT, OAuth"},
            {"label": "File upload", "description": "S3, local storage"},
            {"label": "Real-time", "description": "WebSocket, SSE"}
        ],
        "multiSelect": True
    },
    {
        "question": "Select database type",
        "header": "Database",
        "options": [
            {"label": "PostgreSQL (recommended)", "description": "Relational, stable"},
            {"label": "SQLite", "description": "Simple, file-based"},
            {"label": "MongoDB", "description": "NoSQL, flexible schema"}
        ],
        "multiSelect": False
    }
])
```

### Architecture Options

```python
# API architecture
AskUserQuestion(questions=[
    {
        "question": "Select API architecture",
        "header": "API Arch",
        "options": [
            {"label": "REST (recommended)", "description": "Standard, caching friendly"},
            {"label": "GraphQL", "description": "Flexible queries, client-driven"},
            {"label": "gRPC", "description": "High performance, microservices"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select database pattern",
        "header": "DB Pattern",
        "options": [
            {"label": "Repository Pattern (recommended)", "description": "Abstraction, testable"},
            {"label": "Active Record", "description": "Simple, direct ORM usage"},
            {"label": "CQRS", "description": "Read/write separation, complex"}
        ],
        "multiSelect": False
    }
])
```

---

## ML/DL (`ml-dl`)

### Requirements Interview

```python
AskUserQuestion(questions=[
    {
        "question": "What type of ML task?",
        "header": "ML Task",
        "options": [
            {"label": "Classification", "description": "Image, text classification"},
            {"label": "Regression", "description": "Numeric prediction"},
            {"label": "Generative", "description": "Image, text generation"},
            {"label": "Recommendation", "description": "Personalized recommendations"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select experiment management tool",
        "header": "Experiment",
        "options": [
            {"label": "MLflow (recommended)", "description": "Experiment tracking, model management"},
            {"label": "Weights & Biases", "description": "Visualization, collaboration"},
            {"label": "None", "description": "Manual management"}
        ],
        "multiSelect": False
    }
])
```

### Architecture Options

```python
# ML pipeline architecture
AskUserQuestion(questions=[
    {
        "question": "Select training pipeline structure",
        "header": "Pipeline",
        "options": [
            {"label": "Single script (recommended)", "description": "Simple, prototype"},
            {"label": "Hydra + Config", "description": "Config separation, experiment management"},
            {"label": "Lightning", "description": "Structured, less boilerplate"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select serving architecture",
        "header": "Serving",
        "options": [
            {"label": "FastAPI (recommended)", "description": "Simple, fast deployment"},
            {"label": "TorchServe", "description": "PyTorch native"},
            {"label": "Triton", "description": "High performance, GPU optimized"}
        ],
        "multiSelect": False
    }
])
```

---

## Development Environment Interview

### 3.1 Language & Build

```python
AskUserQuestion(questions=[
    {
        "question": "Which language for this project?",
        "header": "Language",
        "options": [
            {"label": "TypeScript (recommended)", "description": "Type safety, IDE support"},
            {"label": "Vanilla JavaScript", "description": "Simple, quick start"},
            {"label": "Python", "description": "Backend/ML projects"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select build tool",
        "header": "Build",
        "options": [
            {"label": "Vite (recommended)", "description": "Fast dev, HMR support"},
            {"label": "None", "description": "Simple project, CDN usage"},
            {"label": "Webpack", "description": "Complex configuration needs"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select package manager",
        "header": "Package",
        "options": [
            {"label": "pnpm (recommended)", "description": "Fast, disk efficient"},
            {"label": "npm", "description": "Default package manager"},
            {"label": "uv (Python)", "description": "Python projects"}
        ],
        "multiSelect": False
    }
])
```

### 3.2 Test Strategy

```python
AskUserQuestion(questions=[
    {
        "question": "Select unit test framework",
        "header": "Unit Test",
        "options": [
            {"label": "Vitest (recommended)", "description": "Vite compatible, fast"},
            {"label": "Jest", "description": "React standard"},
            {"label": "pytest (Python)", "description": "Python projects"},
            {"label": "Not needed", "description": "Skip tests"}
        ],
        "multiSelect": False
    },
    {
        "question": "Select E2E test framework",
        "header": "E2E",
        "options": [
            {"label": "Not needed", "description": "Unit tests only"},
            {"label": "Playwright (recommended)", "description": "Fast, reliable"},
            {"label": "Cypress", "description": "Intuitive UI"}
        ],
        "multiSelect": False
    }
])
```

### 3.3 Quality Tools

```python
AskUserQuestion(questions=[
    {
        "question": "Select code quality tools (multi-select)",
        "header": "Quality",
        "options": [
            {"label": "ESLint + Prettier (recommended)", "description": "Linting + formatting"},
            {"label": "ESLint only", "description": "Linting only"},
            {"label": "Ruff (Python)", "description": "Python linter/formatter"},
            {"label": "None", "description": "Skip quality tools"}
        ],
        "multiSelect": True
    }
])
```

### 3.4 C4 Workflow

```python
AskUserQuestion(questions=[
    {
        "question": "Where to set checkpoints?",
        "header": "Checkpoint",
        "options": [
            {"label": "Per phase (recommended)", "description": "Supervisor review at each phase"},
            {"label": "Per feature", "description": "Review at major features"},
            {"label": "None", "description": "Review only at the end"}
        ],
        "multiSelect": False
    },
    {
        "question": "Task granularity?",
        "header": "Task Size",
        "options": [
            {"label": "As PRD (recommended)", "description": "Document checklist items as-is"},
            {"label": "Smaller", "description": "Split into substeps"},
            {"label": "Larger", "description": "Merge related items"}
        ],
        "multiSelect": False
    },
    {
        "question": "Auto-execution scope?",
        "header": "Scope",
        "options": [
            {"label": "Phase 1 only", "description": "Auto up to prototype"},
            {"label": "Phase 1~2", "description": "Auto up to basic features"},
            {"label": "All", "description": "Auto-execute all phases"}
        ],
        "multiSelect": False
    }
])
```
