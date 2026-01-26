"""DoD Parser - Parse Definition of Done strings into structured checklist items.

Converts unstructured DoD text into DoDItem objects for progress tracking.

Supported formats:
- Markdown checklist: "- [ ] item" or "- [x] item"
- Numbered list: "1. item"
- Bullet points: "• item" or "* item"
- Plain text with newlines
"""

import re
from typing import Literal

from c4.models.ddd import DoDItem

# Category keywords for automatic classification
CATEGORY_KEYWORDS: dict[str, list[str]] = {
    "test": [
        "test",
        "테스트",
        "검증",
        "assert",
        "coverage",
        "커버리지",
        "pytest",
        "unittest",
    ],
    "gate": [
        "lint",
        "ruff",
        "mypy",
        "typecheck",
        "타입",
        "통과",
        "pass",
        "검사",
        "validation",
    ],
    "review": [
        "review",
        "리뷰",
        "검토",
        "확인",
        "승인",
        "approve",
    ],
    "impl": [
        "구현",
        "implement",
        "create",
        "생성",
        "add",
        "추가",
        "작성",
        "write",
        "build",
    ],
}


def _classify_item(text: str) -> Literal["impl", "test", "gate", "review"]:
    """Classify DoD item by category based on keywords."""
    text_lower = text.lower()

    # Check each category
    for category, keywords in CATEGORY_KEYWORDS.items():
        for keyword in keywords:
            if keyword in text_lower:
                return category  # type: ignore

    # Default to implementation
    return "impl"


def _is_completed(text: str) -> bool:
    """Check if item is marked as completed."""
    # Markdown checkbox completed
    if re.match(r"^\s*-\s*\[x\]", text, re.IGNORECASE):
        return True
    # Strikethrough
    if text.strip().startswith("~~") and text.strip().endswith("~~"):
        return True
    # Checkmark emoji
    if "✓" in text or "✅" in text or "☑" in text:
        return True
    return False


def _extract_text(line: str) -> str:
    """Extract clean text from a DoD line."""
    # Remove markdown checkbox
    line = re.sub(r"^\s*-\s*\[[x ]\]\s*", "", line, flags=re.IGNORECASE)
    # Remove numbered list
    line = re.sub(r"^\s*\d+\.\s*", "", line)
    # Remove bullet points
    line = re.sub(r"^\s*[•\-\*]\s*", "", line)
    # Remove strikethrough
    line = re.sub(r"^~~(.+)~~$", r"\1", line.strip())
    # Remove emoji checkmarks
    line = re.sub(r"[✓✅☑]\s*", "", line)

    return line.strip()


def parse_dod(dod: str) -> list[DoDItem]:
    """Parse DoD string into structured DoDItem list.

    Args:
        dod: Definition of Done string (markdown, plain text, etc.)

    Returns:
        List of DoDItem objects with text, completed status, and category

    Example:
        >>> dod = '''
        ... - [ ] UserService.register() 구현
        ... - [x] test_register_success 통과
        ... - [ ] lint 통과
        ... '''
        >>> items = parse_dod(dod)
        >>> items[0].text
        'UserService.register() 구현'
        >>> items[0].category
        'impl'
        >>> items[1].completed
        True
    """
    items: list[DoDItem] = []

    # Split into lines
    lines = dod.strip().split("\n")

    for line in lines:
        line = line.strip()
        if not line:
            continue

        # Extract text and metadata
        text = _extract_text(line)
        if not text:
            continue

        completed = _is_completed(line)
        category = _classify_item(text)

        items.append(DoDItem(text=text, completed=completed, category=category))

    return items


def format_dod(items: list[DoDItem]) -> str:
    """Format DoDItem list back to markdown string.

    Args:
        items: List of DoDItem objects

    Returns:
        Markdown formatted DoD string
    """
    lines = []
    for item in items:
        checkbox = "[x]" if item.completed else "[ ]"
        lines.append(f"- {checkbox} {item.text}")
    return "\n".join(lines)


def update_dod_item(
    items: list[DoDItem], text_pattern: str, completed: bool
) -> list[DoDItem]:
    """Update completion status of matching DoD items.

    Args:
        items: List of DoDItem objects
        text_pattern: Substring to match in item text
        completed: New completion status

    Returns:
        Updated list of DoDItem objects
    """
    updated = []
    for item in items:
        if text_pattern.lower() in item.text.lower():
            updated.append(DoDItem(text=item.text, completed=completed, category=item.category))
        else:
            updated.append(item)
    return updated


def get_completion_stats(items: list[DoDItem]) -> dict[str, int]:
    """Get completion statistics by category.

    Returns:
        Dictionary with counts: {category: (completed, total)}
    """
    stats: dict[str, dict[str, int]] = {}

    for item in items:
        if item.category not in stats:
            stats[item.category] = {"completed": 0, "total": 0}
        stats[item.category]["total"] += 1
        if item.completed:
            stats[item.category]["completed"] += 1

    return {
        cat: data["completed"]
        for cat, data in stats.items()
    }


def get_overall_completion(items: list[DoDItem]) -> tuple[int, int, float]:
    """Get overall completion statistics.

    Returns:
        Tuple of (completed, total, percentage)
    """
    if not items:
        return 0, 0, 0.0

    total = len(items)
    completed = sum(1 for item in items if item.completed)
    percentage = (completed / total) * 100

    return completed, total, percentage


def validate_dod_requirements(items: list[DoDItem]) -> list[str]:
    """Validate DoD meets minimum requirements from DDD-CLEANCODE guide.

    Minimum requirements:
    - At least 1 implementation item
    - At least 1 test item
    - At least 1 gate item (lint, typecheck, etc.)

    Returns:
        List of validation errors (empty if valid)
    """
    errors = []

    categories = {item.category for item in items}

    if "impl" not in categories:
        errors.append("DoD missing implementation items")
    if "test" not in categories:
        errors.append("DoD missing test items")
    if "gate" not in categories:
        errors.append("DoD missing quality gate items (lint, typecheck)")

    # Check for specific test requirements
    test_items = [item for item in items if item.category == "test"]
    test_texts = " ".join(item.text.lower() for item in test_items)

    has_success_test = any(
        kw in test_texts for kw in ["success", "성공", "happy", "정상"]
    )
    has_failure_test = any(
        kw in test_texts for kw in ["fail", "실패", "error", "에러", "exception"]
    )
    has_boundary_test = any(
        kw in test_texts for kw in ["boundary", "edge", "경계", "limit", "max", "min"]
    )

    if test_items and not has_success_test:
        errors.append("DoD missing success case test")
    if test_items and not has_failure_test:
        errors.append("DoD missing failure case test")
    if test_items and not has_boundary_test:
        errors.append("DoD missing boundary case test")

    return errors


def generate_standard_dod(
    impl_items: list[str],
    test_items: list[str],
    gates: list[str] | None = None,
) -> str:
    """Generate a standard DoD string from components.

    Args:
        impl_items: Implementation items
        test_items: Test items
        gates: Quality gates (defaults to ["lint 통과", "기존 테스트 유지"])

    Returns:
        Formatted DoD markdown string
    """
    if gates is None:
        gates = ["lint 통과", "기존 테스트 유지"]

    lines = []

    # Implementation items
    for item in impl_items:
        lines.append(f"- [ ] {item}")

    # Test items
    for item in test_items:
        lines.append(f"- [ ] {item}")

    # Gate items
    for item in gates:
        lines.append(f"- [ ] {item}")

    return "\n".join(lines)
