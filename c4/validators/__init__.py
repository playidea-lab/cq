"""C4 Validators - DDD-CLEANCODE validation tools.

This package provides validators for:
- BoundaryMap: Import rules and layer constraints
- WorkBreakdown: Task size limits
"""

from .boundary import (
    BoundaryValidationResult,
    ImportViolation,
    format_violations_report,
    generate_import_check_script,
    validate_boundary,
    validate_file_imports,
)
from .work_breakdown import (
    DEFAULT_CRITERIA,
    SplitRecommendation,
    WorkBreakdownResult,
    analyze_task_size,
    format_breakdown_report,
    suggest_task_splits,
)

__all__ = [
    # Boundary validation
    "BoundaryValidationResult",
    "ImportViolation",
    "format_violations_report",
    "generate_import_check_script",
    "validate_boundary",
    "validate_file_imports",
    # Work breakdown
    "DEFAULT_CRITERIA",
    "SplitRecommendation",
    "WorkBreakdownResult",
    "analyze_task_size",
    "format_breakdown_report",
    "suggest_task_splits",
]
