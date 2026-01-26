"""Boundary Validator - Enforce DDD layer constraints and import rules.

This module validates that code respects clean architecture boundaries:
- Layer dependencies (domain should not import from infra)
- Forbidden imports (e.g., no sqlalchemy in domain layer)
- Public export enforcement (single export point)
"""

import ast
from pathlib import Path
from typing import NamedTuple

from c4.models.ddd import BoundaryMap

# Standard library modules (not exhaustive, but covers common ones)
STDLIB_MODULES = {
    "abc",
    "asyncio",
    "collections",
    "contextlib",
    "copy",
    "dataclasses",
    "datetime",
    "decimal",
    "enum",
    "functools",
    "hashlib",
    "io",
    "itertools",
    "json",
    "logging",
    "math",
    "os",
    "pathlib",
    "pickle",
    "random",
    "re",
    "secrets",
    "shutil",
    "signal",
    "socket",
    "sqlite3",
    "string",
    "subprocess",
    "sys",
    "tempfile",
    "threading",
    "time",
    "typing",
    "typing_extensions",
    "unittest",
    "urllib",
    "uuid",
    "warnings",
    "weakref",
}

# Layer hierarchy (lower can't import from higher)
LAYER_HIERARCHY = {
    "domain": 0,  # Core business logic - no external deps
    "app": 1,  # Application services - can import domain
    "infra": 2,  # Infrastructure - can import domain, app
    "api": 3,  # API layer - can import all
    "presentation": 3,  # Same level as API
}


class ImportViolation(NamedTuple):
    """Import violation details."""

    file: str
    line: int
    module: str
    reason: str


class BoundaryValidationResult(NamedTuple):
    """Result of boundary validation."""

    valid: bool
    violations: list[ImportViolation]
    warnings: list[str]


def _extract_imports(file_path: Path) -> list[tuple[int, str]]:
    """Extract all imports from a Python file.

    Returns:
        List of (line_number, module_name) tuples
    """
    imports = []

    try:
        source = file_path.read_text()
        tree = ast.parse(source)

        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    imports.append((node.lineno, alias.name))
            elif isinstance(node, ast.ImportFrom):
                if node.module:
                    imports.append((node.lineno, node.module))
    except (SyntaxError, UnicodeDecodeError) as e:
        # Log warning but don't fail
        imports.append((-1, f"__parse_error__: {e}"))

    return imports


def _is_stdlib(module: str) -> bool:
    """Check if module is from standard library."""
    root_module = module.split(".")[0]
    return root_module in STDLIB_MODULES


def _is_allowed_import(
    module: str, allowed: list[str], forbidden: list[str]
) -> tuple[bool, str | None]:
    """Check if import is allowed based on rules.

    Returns:
        Tuple of (is_allowed, reason_if_not_allowed)
    """
    root_module = module.split(".")[0]

    # Check explicit forbidden first (takes precedence)
    for forbidden_pattern in forbidden:
        if forbidden_pattern.lower() in module.lower():
            return False, f"Forbidden import: {forbidden_pattern}"

    # Check if in allowed list
    for allowed_pattern in allowed:
        # Special patterns
        if allowed_pattern == "stdlib" and _is_stdlib(module):
            return True, None
        if allowed_pattern == "pydantic" and root_module == "pydantic":
            return True, None
        if allowed_pattern == "typing" and root_module in ("typing", "typing_extensions"):
            return True, None

        # Direct match or prefix match
        if module.startswith(allowed_pattern) or root_module == allowed_pattern:
            return True, None

    return False, f"Import not in allowed list: {module}"


def validate_file_imports(
    file_path: Path,
    boundary_map: BoundaryMap,
) -> list[ImportViolation]:
    """Validate imports in a single file against boundary rules.

    Args:
        file_path: Path to Python file
        boundary_map: Boundary constraints

    Returns:
        List of import violations
    """
    violations = []

    if not file_path.exists():
        return violations

    if not file_path.suffix == ".py":
        return violations

    imports = _extract_imports(file_path)

    for line, module in imports:
        if module.startswith("__parse_error__"):
            violations.append(
                ImportViolation(
                    file=str(file_path),
                    line=line,
                    module=module,
                    reason="Failed to parse file",
                )
            )
            continue

        allowed, reason = _is_allowed_import(
            module, boundary_map.allowed_imports, boundary_map.forbidden_imports
        )

        if not allowed and reason:
            violations.append(
                ImportViolation(
                    file=str(file_path),
                    line=line,
                    module=module,
                    reason=reason,
                )
            )

    return violations


def validate_layer_hierarchy(
    source_layer: str,
    target_module: str,
    project_root: Path,
) -> ImportViolation | None:
    """Validate import doesn't violate layer hierarchy.

    Args:
        source_layer: Layer of the importing file
        target_module: Module being imported
        project_root: Project root directory

    Returns:
        ImportViolation if hierarchy violated, None otherwise
    """
    # Infer target layer from module path
    target_layer = None

    # Check for layer keywords in module path
    module_lower = target_module.lower()
    for layer in LAYER_HIERARCHY:
        if f".{layer}." in module_lower or module_lower.startswith(f"{layer}."):
            target_layer = layer
            break

    if target_layer is None:
        return None  # Can't determine layer, skip check

    source_level = LAYER_HIERARCHY.get(source_layer, 999)
    target_level = LAYER_HIERARCHY.get(target_layer, 999)

    # Lower layers shouldn't import from higher layers
    if source_level < target_level:
        return ImportViolation(
            file="",  # Filled in by caller
            line=0,
            module=target_module,
            reason=f"Layer violation: {source_layer} cannot import from {target_layer}",
        )

    return None


def validate_boundary(
    files: list[Path],
    boundary_map: BoundaryMap,
    project_root: Path | None = None,
) -> BoundaryValidationResult:
    """Validate multiple files against boundary constraints.

    Args:
        files: List of files to validate
        boundary_map: Boundary constraints
        project_root: Project root (for layer detection)

    Returns:
        BoundaryValidationResult with violations and warnings
    """
    all_violations = []
    warnings = []

    for file_path in files:
        violations = validate_file_imports(file_path, boundary_map)
        all_violations.extend(violations)

        # Also check layer hierarchy
        if project_root:
            imports = _extract_imports(file_path)
            for line, module in imports:
                violation = validate_layer_hierarchy(
                    boundary_map.target_layer, module, project_root
                )
                if violation:
                    all_violations.append(
                        ImportViolation(
                            file=str(file_path),
                            line=line,
                            module=violation.module,
                            reason=violation.reason,
                        )
                    )

    # Add warnings for non-critical issues
    if not boundary_map.public_export:
        warnings.append("No public_export defined - API exposure point unknown")

    return BoundaryValidationResult(
        valid=len(all_violations) == 0,
        violations=all_violations,
        warnings=warnings,
    )


def generate_import_check_script(boundary_map: BoundaryMap, output_path: Path) -> None:
    """Generate a standalone import check script.

    This can be used as a quality gate command:
    `uv run python scripts/check_imports.py src/auth/`

    Args:
        boundary_map: Boundary constraints
        output_path: Where to write the script
    """
    allowed_str = ", ".join(f'"{a}"' for a in boundary_map.allowed_imports)
    forbidden_str = ", ".join(f'"{f}"' for f in boundary_map.forbidden_imports)

    script = f'''#!/usr/bin/env python3
"""Auto-generated import checker for {boundary_map.target_domain} domain."""

import sys
from pathlib import Path

# Add project root to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from c4.validators.boundary import validate_boundary
from c4.models.ddd import BoundaryMap

BOUNDARY = BoundaryMap(
    target_domain="{boundary_map.target_domain}",
    target_layer="{boundary_map.target_layer}",
    allowed_imports=[{allowed_str}],
    forbidden_imports=[{forbidden_str}],
    public_export={repr(boundary_map.public_export)},
)


def main():
    if len(sys.argv) < 2:
        print("Usage: check_imports.py <directory>")
        sys.exit(1)

    target_dir = Path(sys.argv[1])
    if not target_dir.exists():
        print(f"Directory not found: {{target_dir}}")
        sys.exit(1)

    files = list(target_dir.rglob("*.py"))
    result = validate_boundary(files, BOUNDARY, target_dir.parent)

    if result.violations:
        print(f"❌ Found {{len(result.violations)}} boundary violations:")
        for v in result.violations:
            print(f"  {{v.file}}:{{v.line}} - {{v.module}}: {{v.reason}}")
        sys.exit(1)

    if result.warnings:
        print("⚠️ Warnings:")
        for w in result.warnings:
            print(f"  {{w}}")

    print(f"✅ All {{len(files)}} files pass boundary checks")
    sys.exit(0)


if __name__ == "__main__":
    main()
'''

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(script)
    output_path.chmod(0o755)


def format_violations_report(violations: list[ImportViolation]) -> str:
    """Format violations into a readable report."""
    if not violations:
        return "✅ No boundary violations found"

    lines = [f"❌ Found {len(violations)} boundary violations:", ""]

    # Group by file
    by_file: dict[str, list[ImportViolation]] = {}
    for v in violations:
        if v.file not in by_file:
            by_file[v.file] = []
        by_file[v.file].append(v)

    for file, file_violations in sorted(by_file.items()):
        lines.append(f"📁 {file}")
        for v in file_violations:
            lines.append(f"   Line {v.line}: {v.module}")
            lines.append(f"   └─ {v.reason}")
        lines.append("")

    return "\n".join(lines)
