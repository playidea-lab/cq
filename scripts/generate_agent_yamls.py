#!/usr/bin/env python3
"""Generate YAML files for agent graph from existing DOMAIN_AGENT_MAP."""

from pathlib import Path

# Output directory
OUTPUT_DIR = Path(".c4/agents")

# Domain definitions
DOMAINS = {
    "web-frontend": {
        "name": "Web Frontend",
        "description": "React/Vue/CSS, component development, accessibility, responsive design",
        "skills": ["typescript-coding", "react-development", "css-styling"],
        "workflow": [
            ("frontend-developer", "Implement frontend components and features"),
            ("test-automator", "Write unit and integration tests"),
            ("code-reviewer", "Review code for quality and best practices"),
        ],
    },
    "web-backend": {
        "name": "Web Backend",
        "description": "API design, database schemas, Python/Node implementation",
        "skills": ["python-coding", "api-design", "database-design"],
        "workflow": [
            ("backend-architect", "Design API and database schemas"),
            ("python-pro", "Implement backend logic"),
            ("test-automator", "Write API and integration tests"),
            ("code-reviewer", "Review code for quality and security"),
        ],
    },
    "fullstack": {
        "name": "Full Stack",
        "description": "Full-stack development with both frontend and backend",
        "skills": ["python-coding", "typescript-coding", "api-design"],
        "workflow": [
            ("backend-architect", "Design overall architecture"),
            ("frontend-developer", "Implement frontend components"),
            ("test-automator", "Write end-to-end tests"),
            ("code-reviewer", "Review full stack integration"),
        ],
    },
    "ml-dl": {
        "name": "Machine Learning & Deep Learning",
        "description": "ML model development, training pipelines, data processing",
        "skills": ["python-coding", "ml-modeling", "data-processing"],
        "workflow": [
            ("ml-engineer", "Design and train ML models"),
            ("python-pro", "Optimize Python code"),
            ("test-automator", "Write model validation tests"),
        ],
    },
    "mobile-app": {
        "name": "Mobile Application",
        "description": "React Native/Flutter mobile app development",
        "skills": ["mobile-development", "typescript-coding"],
        "workflow": [
            ("mobile-developer", "Implement mobile features"),
            ("test-automator", "Write mobile tests"),
            ("code-reviewer", "Review mobile patterns"),
        ],
    },
    "infra": {
        "name": "Infrastructure",
        "description": "Cloud infrastructure, Terraform, Kubernetes",
        "skills": ["cloud-infrastructure", "devops"],
        "workflow": [
            ("cloud-architect", "Design infrastructure"),
            ("deployment-engineer", "Implement and deploy"),
        ],
    },
    "library": {
        "name": "Library/Package",
        "description": "Reusable library or package development",
        "skills": ["python-coding", "api-design", "documentation"],
        "workflow": [
            ("python-pro", "Implement library code"),
            ("api-documenter", "Write API documentation"),
            ("test-automator", "Write comprehensive tests"),
            ("code-reviewer", "Review public API"),
        ],
    },
    "firmware": {
        "name": "Firmware/Embedded",
        "description": "Embedded systems and firmware development",
        "skills": ["embedded-development"],
        "workflow": [
            ("general-purpose", "Implement firmware"),
            ("test-automator", "Write hardware tests"),
        ],
    },
    "unknown": {
        "name": "Unknown Domain",
        "description": "Default fallback for unknown domains",
        "skills": [],
        "workflow": [
            ("general-purpose", "Handle general tasks"),
            ("code-reviewer", "Review changes"),
        ],
    },
    "data-science": {
        "name": "Data Science",
        "description": "Data analysis, visualization, Jupyter notebooks",
        "skills": ["python-coding", "data-analysis", "visualization"],
        "workflow": [
            ("data-scientist", "Analyze data and build models"),
            ("python-pro", "Optimize data pipelines"),
            ("test-automator", "Write data validation tests"),
        ],
    },
    "devops": {
        "name": "DevOps",
        "description": "CI/CD, monitoring, infrastructure automation",
        "skills": ["devops", "cloud-infrastructure", "security"],
        "workflow": [
            ("deployment-engineer", "Implement CI/CD pipelines"),
            ("cloud-architect", "Design infrastructure"),
            ("security-auditor", "Review security"),
        ],
    },
    "api": {
        "name": "API Development",
        "description": "REST/GraphQL API design and documentation",
        "skills": ["api-design", "documentation"],
        "workflow": [
            ("backend-architect", "Design API endpoints"),
            ("api-documenter", "Write API documentation"),
            ("test-automator", "Write API tests"),
        ],
    },
}

# Agent persona definitions
# Format: (target_agent, when, passes, weight)
AGENTS = {
    "frontend-developer": {
        "name": "Frontend Developer",
        "role": "Frontend specialist",
        "expertise": "React, Vue, CSS, accessibility, responsive design",
        "skills": ["typescript-coding", "react-development", "css-styling"],
        "hands_off_to": [("test-automator", "UI complete", "Component specs", 0.85)],
    },
    "backend-architect": {
        "name": "Backend Architect",
        "role": "Backend design lead",
        "expertise": "API design, database schemas, microservices",
        "skills": ["python-coding", "api-design", "database-design"],
        "hands_off_to": [("python-pro", "Architecture designed", "API specs", 0.9)],
    },
    "python-pro": {
        "name": "Python Professional",
        "role": "Python expert",
        "expertise": "Idiomatic Python, performance, async/await",
        "skills": ["python-coding"],
        "hands_off_to": [("test-automator", "Implementation complete", "Code changes", 0.85)],
    },
    "test-automator": {
        "name": "Test Automator",
        "role": "Testing specialist",
        "expertise": "Unit tests, integration tests, CI/CD",
        "skills": ["testing"],
        "hands_off_to": [("code-reviewer", "Tests written", "Test results", 0.8)],
    },
    "code-reviewer": {
        "name": "Code Reviewer",
        "role": "Quality assurance",
        "expertise": "Code review, best practices, security",
        "skills": ["code-review"],
        "hands_off_to": [],
    },
    "ml-engineer": {
        "name": "ML Engineer",
        "role": "Machine learning specialist",
        "expertise": "Model training, PyTorch/TensorFlow, MLOps",
        "skills": ["python-coding", "ml-modeling"],
        "hands_off_to": [("python-pro", "Model trained", "Model artifacts", 0.85)],
    },
    "mobile-developer": {
        "name": "Mobile Developer",
        "role": "Mobile specialist",
        "expertise": "React Native, Flutter, mobile patterns",
        "skills": ["mobile-development", "typescript-coding"],
        "hands_off_to": [("test-automator", "Mobile feature complete", "App changes", 0.85)],
    },
    "cloud-architect": {
        "name": "Cloud Architect",
        "role": "Infrastructure specialist",
        "expertise": "AWS/GCP/Azure, Terraform, Kubernetes",
        "skills": ["cloud-infrastructure"],
        "hands_off_to": [("deployment-engineer", "Infrastructure designed", "IaC files", 0.9)],
    },
    "deployment-engineer": {
        "name": "Deployment Engineer",
        "role": "DevOps specialist",
        "expertise": "CI/CD, Docker, Kubernetes, monitoring",
        "skills": ["devops"],
        "hands_off_to": [],
    },
    "api-documenter": {
        "name": "API Documenter",
        "role": "Documentation specialist",
        "expertise": "OpenAPI, SDK generation, developer docs",
        "skills": ["documentation", "api-design"],
        "hands_off_to": [("test-automator", "Documentation complete", "API docs", 0.75)],
    },
    "general-purpose": {
        "name": "General Purpose Agent",
        "role": "Generalist",
        "expertise": "Various tasks, research, exploration",
        "skills": [],
        "hands_off_to": [("code-reviewer", "Task complete", "Work summary", 0.7)],
    },
    "debugger": {
        "name": "Debugger",
        "role": "Debugging specialist",
        "expertise": "Bug analysis, root cause, fixes",
        "skills": ["debugging"],
        "hands_off_to": [("test-automator", "Bug fixed", "Fix details", 0.85)],
    },
    "performance-engineer": {
        "name": "Performance Engineer",
        "role": "Performance specialist",
        "expertise": "Profiling, optimization, benchmarking",
        "skills": ["performance-optimization"],
        "hands_off_to": [("test-automator", "Optimization complete", "Perf results", 0.8)],
    },
    "security-auditor": {
        "name": "Security Auditor",
        "role": "Security specialist",
        "expertise": "Vulnerability assessment, security review",
        "skills": ["security-audit"],
        "hands_off_to": [("code-reviewer", "Audit complete", "Audit report", 0.85)],
    },
    "database-optimizer": {
        "name": "Database Optimizer",
        "role": "Database specialist",
        "expertise": "Query optimization, indexing, migrations",
        "skills": ["database-design"],
        "hands_off_to": [("test-automator", "Optimization complete", "Query stats", 0.8)],
    },
    "code-refactorer": {
        "name": "Code Refactorer",
        "role": "Refactoring specialist",
        "expertise": "Code cleanup, restructuring, patterns",
        "skills": ["refactoring"],
        "hands_off_to": [("test-automator", "Refactoring complete", "Changes summary", 0.85)],
    },
    "graphql-architect": {
        "name": "GraphQL Architect",
        "role": "GraphQL specialist",
        "expertise": "Schema design, resolvers, federation",
        "skills": ["graphql-design", "api-design"],
        "hands_off_to": [("test-automator", "Schema designed", "Schema files", 0.85)],
    },
    "payment-integration": {
        "name": "Payment Integration Specialist",
        "role": "Payment specialist",
        "expertise": "Stripe, PayPal, billing, PCI compliance",
        "skills": ["payment-integration"],
        "hands_off_to": [("security-auditor", "Integration complete", "Payment config", 0.9)],
    },
    "data-engineer": {
        "name": "Data Engineer",
        "role": "Data pipeline specialist",
        "expertise": "ETL, data warehouses, Spark, Airflow",
        "skills": ["data-engineering"],
        "hands_off_to": [("test-automator", "Pipeline complete", "Pipeline code", 0.8)],
    },
    "data-scientist": {
        "name": "Data Scientist",
        "role": "Data analysis specialist",
        "expertise": "Analysis, visualization, modeling",
        "skills": ["data-analysis", "python-coding"],
        "hands_off_to": [("python-pro", "Analysis complete", "Analysis results", 0.75)],
    },
    "devops-troubleshooter": {
        "name": "DevOps Troubleshooter",
        "role": "Troubleshooting specialist",
        "expertise": "Production debugging, monitoring, alerts",
        "skills": ["devops", "debugging"],
        "hands_off_to": [("deployment-engineer", "Issue identified", "Issue report", 0.85)],
    },
    "incident-responder": {
        "name": "Incident Responder",
        "role": "Incident management specialist",
        "expertise": "Incident response, postmortems, SRE",
        "skills": ["incident-response", "devops"],
        "hands_off_to": [("devops-troubleshooter", "Incident contained", "Incident status", 0.9)],
    },
}

# Task type override rules
TASK_OVERRIDES = {
    "debug": ("debugger", ["debug", "fix bug"]),
    "debugging": ("debugger", ["debugging"]),
    "fix-bug": ("debugger", ["fix-bug", "fix bug"]),
    "performance": ("performance-engineer", ["performance", "slow"]),
    "optimization": ("performance-engineer", ["optimization", "optimize"]),
    "profiling": ("performance-engineer", ["profiling", "profile"]),
    "security": ("security-auditor", ["security", "vulnerability"]),
    "vulnerability": ("security-auditor", ["vulnerability", "CVE"]),
    "audit": ("security-auditor", ["audit"]),
    "database": ("database-optimizer", ["database", "query"]),
    "query": ("database-optimizer", ["query", "SQL"]),
    "migration": ("database-optimizer", ["migration"]),
    "docs": ("api-documenter", ["docs", "documentation"]),
    "documentation": ("api-documenter", ["documentation"]),
    "readme": ("api-documenter", ["readme", "README"]),
    "refactor": ("code-refactorer", ["refactor"]),
    "cleanup": ("code-refactorer", ["cleanup", "clean up"]),
    "restructure": ("code-refactorer", ["restructure"]),
    "test": ("test-automator", ["test"]),
    "testing": ("test-automator", ["testing"]),
    "coverage": ("test-automator", ["coverage"]),
    "deploy": ("deployment-engineer", ["deploy"]),
    "ci-cd": ("deployment-engineer", ["ci-cd", "CI/CD"]),
    "pipeline": ("deployment-engineer", ["pipeline"]),
    "graphql": ("graphql-architect", ["graphql", "GraphQL"]),
    "schema": ("graphql-architect", ["schema"]),
    "payment": ("payment-integration", ["payment", "billing"]),
    "stripe": ("payment-integration", ["stripe", "Stripe"]),
    "billing": ("payment-integration", ["billing"]),
    "data-pipeline": ("data-engineer", ["data-pipeline", "ETL"]),
    "etl": ("data-engineer", ["etl", "ETL"]),
    "analytics": ("data-scientist", ["analytics"]),
    "api-design": ("backend-architect", ["api-design", "API design"]),
    "data-analysis": ("data-scientist", ["data-analysis", "analysis"]),
    "monitoring": ("devops-troubleshooter", ["monitoring"]),
    "incident": ("incident-responder", ["incident"]),
    "infra-setup": ("cloud-architect", ["infra-setup", "infrastructure"]),
    "notebook": ("data-scientist", ["notebook", "jupyter"]),
}


def write_domain_yaml(domain_id: str, domain_info: dict) -> None:
    """Write a domain YAML file."""
    domains_dir = OUTPUT_DIR / "domains"
    domains_dir.mkdir(parents=True, exist_ok=True)

    filepath = domains_dir / f"{domain_id}.yaml"

    workflow_items = []
    for i, (agent, purpose) in enumerate(domain_info["workflow"], start=1):
        workflow_items.append(f"""    - step: {i}
      role: {'primary' if i == 1 else 'support'}
      select:
        by: agent
        prefer_agent: {agent}
      purpose: {purpose}""")

    skills_core = domain_info.get("skills", [])
    skills_section = ""
    if skills_core:
        skills_section = f"""  required_skills:
    core:
{chr(10).join(f'      - {s}' for s in skills_core)}"""
    else:
        skills_section = "  required_skills:\n    core: []"

    content = f"""domain:
  id: {domain_id}
  name: {domain_info['name']}
  description: {domain_info['description']}
{skills_section}
  workflow:
{chr(10).join(workflow_items)}
"""

    filepath.write_text(content)
    print(f"Created: {filepath}")


def write_agent_yaml(agent_id: str, agent_info: dict) -> None:
    """Write an agent persona YAML file."""
    personas_dir = OUTPUT_DIR / "personas"
    personas_dir.mkdir(parents=True, exist_ok=True)

    filepath = personas_dir / f"{agent_id}.yaml"

    skills = agent_info.get("skills", [])
    skills_section = ""
    if skills:
        skills_section = f"""  skills:
    primary:
{chr(10).join(f'      - {s}' for s in skills)}"""
    else:
        skills_section = "  skills:\n    primary: []"

    hands_off = agent_info.get("hands_off_to", [])
    relationships_section = ""
    if hands_off:
        handoffs = []
        for target, when, passes, weight in hands_off:
            handoffs.append(f"""      - agent: {target}
        when: {when}
        passes: {passes}
        weight: {weight}""")
        relationships_section = f"""  relationships:
    hands_off_to:
{chr(10).join(handoffs)}"""
    else:
        relationships_section = "  relationships:\n    hands_off_to: []"

    content = f"""agent:
  id: {agent_id}
  name: {agent_info['name']}
  persona:
    role: {agent_info['role']}
    expertise: {agent_info['expertise']}
{skills_section}
{relationships_section}
"""

    filepath.write_text(content)
    print(f"Created: {filepath}")


def write_rules_yaml() -> None:
    """Write task override rules YAML file."""
    rules_dir = OUTPUT_DIR / "rules"
    rules_dir.mkdir(parents=True, exist_ok=True)

    filepath = rules_dir / "task-overrides.yaml"

    overrides = []
    for task_type, (agent, keywords) in TASK_OVERRIDES.items():
        overrides.append(f"""  - name: {task_type}-override
    priority: 90
    condition:
      has_keyword:
{chr(10).join(f'        - {kw}' for kw in keywords)}
    action:
      set_primary: {agent}
    reason: {task_type.replace('-', ' ').title()} tasks use {agent}""")

    content = f"""rules:
  overrides:
{chr(10).join(overrides)}
"""

    filepath.write_text(content)
    print(f"Created: {filepath}")


def main():
    """Generate all YAML files."""
    print("Generating domain YAMLs...")
    for domain_id, domain_info in DOMAINS.items():
        write_domain_yaml(domain_id, domain_info)

    print("\nGenerating agent persona YAMLs...")
    for agent_id, agent_info in AGENTS.items():
        write_agent_yaml(agent_id, agent_info)

    print("\nGenerating rules YAML...")
    write_rules_yaml()

    print("\nDone! Generated files:")
    print(f"  - {len(DOMAINS)} domain definitions")
    print(f"  - {len(AGENTS)} agent personas")
    print(f"  - {len(TASK_OVERRIDES)} task override rules")


if __name__ == "__main__":
    main()
