"""
c2 workspace management.

Parses and updates c2_workspace.md files that track project state
across the four domains (Discover, Read, Write, Review).
"""

from __future__ import annotations

import re
from datetime import date
from pathlib import Path

from c4.c2.models import (
    ChangeEntry,
    ClaimEvidence,
    ProjectType,
    ReadingNote,
    ReadingPass,
    Relevance,
    ReviewRecord,
    ReviewReflectionStatus,
    ReviewType,
    SectionState,
    SectionStatus,
    Source,
    SourceStatus,
    SourceType,
    WorkspaceState,
)


def render_workspace(state: WorkspaceState) -> str:
    """Render a WorkspaceState to markdown (c2_workspace.md format).

    Args:
        state: The workspace state to render.

    Returns:
        Markdown string.
    """
    lines: list[str] = []

    lines.append(f"# c2 Workspace - {state.project_name}")
    lines.append("")
    lines.append("## 프로젝트 정보")
    lines.append(f"- **유형**: {state.project_type.value}")
    lines.append(f"- **목표**: {state.goal}")
    lines.append(f"- **생성일**: {state.created_at or date.today()}")
    lines.append(f"- **마지막 세션**: {state.last_session}")
    lines.append("")

    # Discover
    lines.append("## Discover (자료 탐색)")
    lines.append("| # | 자료 | 유형 | 관련도 | 상태 | 비고 |")
    lines.append("|---|------|------|--------|------|------|")
    for i, src in enumerate(state.sources, 1):
        lines.append(
            f"| {i} | {src.title} | {src.type.value} | {src.relevance.value} "
            f"| {src.status.value} | {src.notes} |"
        )
    lines.append("")

    # Read
    lines.append("## Read (읽기 노트)")
    lines.append("| 자료 | 핵심 주장 | 방법/접근 | 우리와의 연결 | 노트 파일 |")
    lines.append("|------|----------|----------|-------------|----------|")
    for note in state.reading_notes:
        claims = "; ".join(note.passes[0].claims[:2]) if note.passes else ""
        method = note.passes[0].method_notes[:40] if note.passes else ""
        conn = note.passes[0].connection_to_project[:40] if note.passes else ""
        lines.append(
            f"| {note.source_title or note.source_id} | {claims} "
            f"| {method} | {conn} | read/{note.source_id}_note.md |"
        )
    lines.append("")

    # Write
    lines.append("## Write (작성 상태)")
    lines.append("| 섹션 | 상태 | 비고 |")
    lines.append("|------|------|------|")
    for sec in state.sections:
        lines.append(f"| {sec.name} | {sec.status.value} | {sec.notes} |")
    lines.append("")

    # Review
    lines.append("## Review (리뷰 이력)")
    lines.append("| 날짜 | 리뷰어 | 유형 | 주요 피드백 | 반영 상태 |")
    lines.append("|------|--------|------|-----------|----------|")
    for rev in state.reviews:
        lines.append(
            f"| {rev.date or ''} | {rev.reviewer} | {rev.type.value} "
            f"| {rev.summary} | {rev.reflection_status.value} |"
        )
    lines.append("")

    # Claim-Evidence
    lines.append("## Claim-Evidence 매핑")
    lines.append("| 주장 | 근거 자료 | 결과/수치 | 위치 |")
    lines.append("|------|----------|----------|------|")
    for ce in state.claim_evidence:
        lines.append(f"| {ce.claim} | {ce.evidence_source} | {ce.result} | {ce.location} |")
    lines.append("")

    # Open questions
    lines.append("## 열린 질문")
    if state.open_questions:
        for q in state.open_questions:
            lines.append(f"- {q}")
    else:
        lines.append("-")
    lines.append("")

    # Changelog
    lines.append("## 변경 이력")
    lines.append("| 날짜 | 도메인 | 작업 | 결정 |")
    lines.append("|------|--------|------|------|")
    for entry in state.changelog:
        lines.append(
            f"| {entry.date or ''} | {entry.domain} | {entry.action} | {entry.decision} |"
        )
    lines.append("")

    return "\n".join(lines)


def create_workspace(
    project_name: str,
    project_type: ProjectType,
    goal: str,
    sections: list[str] | None = None,
) -> WorkspaceState:
    """Create a new workspace state with default sections.

    Args:
        project_name: Name of the project.
        project_type: Type of project.
        goal: One-line goal description.
        sections: Optional list of section names (defaults from project type).

    Returns:
        New WorkspaceState.
    """
    if sections is None:
        if project_type == ProjectType.ACADEMIC_PAPER:
            sections = [
                "abstract",
                "introduction",
                "related_work",
                "method",
                "experiments",
                "discussion",
                "conclusion",
            ]
        elif project_type == ProjectType.PROPOSAL:
            sections = [
                "executive_summary",
                "background",
                "objectives",
                "approach",
                "timeline",
                "budget",
                "expected_outcomes",
            ]
        else:
            sections = []

    return WorkspaceState(
        project_name=project_name,
        project_type=project_type,
        goal=goal,
        created_at=date.today(),
        last_session=f"{date.today()} - 프로젝트 생성",
        sections=[SectionState(name=s) for s in sections],
    )


def save_workspace(state: WorkspaceState, project_dir: Path) -> Path:
    """Save workspace state as c2_workspace.md.

    Args:
        state: Workspace state.
        project_dir: Project directory path.

    Returns:
        Path to the saved file.
    """
    project_dir.mkdir(parents=True, exist_ok=True)
    ws_path = project_dir / "c2_workspace.md"
    ws_path.write_text(render_workspace(state), encoding="utf-8")
    return ws_path


def parse_table_rows(text: str, header_pattern: str) -> list[list[str]]:
    """Parse markdown table rows after a header pattern.

    Args:
        text: Full markdown text.
        header_pattern: Regex to find the table header line.

    Returns:
        List of row cells (list of strings per row).
    """
    rows: list[list[str]] = []
    lines = text.split("\n")
    in_table = False
    skip_separator = False

    for line in lines:
        stripped = line.strip()
        if re.search(header_pattern, stripped):
            in_table = True
            skip_separator = True
            continue
        if in_table and skip_separator and stripped.startswith("|--"):
            skip_separator = False
            continue
        if in_table:
            if stripped.startswith("|") and "|" in stripped[1:]:
                cells = [c.strip() for c in stripped.split("|")[1:-1]]
                if cells and not all(c.startswith("--") for c in cells):
                    rows.append(cells)
            else:
                break

    return rows


# --- Reverse parsing (markdown → model) ---


def _enum_by_value(enum_cls: type, value: str, default=None):
    """Reverse-map an enum by its .value string (e.g. Korean labels).

    Args:
        enum_cls: The Enum class to search.
        value: The .value string to match.
        default: Fallback if no match found.

    Returns:
        Matching enum member, or *default*.
    """
    value = value.strip()
    for member in enum_cls:
        if member.value == value:
            return member
    return default


def _parse_metadata(section_text: str) -> dict[str, str]:
    """Parse ``- **key**: value`` lines from the 프로젝트 정보 section.

    Returns:
        Dict mapping Korean keys to their values.
    """
    result: dict[str, str] = {}
    for line in section_text.split("\n"):
        m = re.match(r"^-\s+\*\*(.+?)\*\*:\s*(.+)$", line.strip())
        if m:
            result[m.group(1).strip()] = m.group(2).strip()
    return result


def _parse_discover(section_text: str) -> list[Source]:
    """Parse the Discover table into Source objects.

    Columns: ``| # | 자료 | 유형 | 관련도 | 상태 | 비고 |``

    Note: ``id``, ``authors``, ``keywords``, ``year``, ``url``, ``discovered_at``
    are NOT stored in the workspace table and cannot be recovered.  We generate
    a slug id from the title.
    """
    rows = _parse_table_from_section(section_text)
    sources: list[Source] = []
    for cells in rows:
        if len(cells) < 6:
            continue
        title = cells[1].strip()
        slug = re.sub(r"[^a-zA-Z0-9가-힣]", "_", title)[:40].strip("_")
        sources.append(
            Source(
                id=slug or f"src_{len(sources)+1}",
                title=title,
                type=_enum_by_value(SourceType, cells[2], SourceType.PAPER),
                relevance=_enum_by_value(Relevance, cells[3], Relevance.MEDIUM),
                status=_enum_by_value(SourceStatus, cells[4], SourceStatus.DISCOVERED),
                notes=cells[5].strip(),
            )
        )
    return sources


def _parse_read(section_text: str) -> list[ReadingNote]:
    """Parse the Read table into ReadingNote objects.

    Columns: ``| 자료 | 핵심 주장 | 방법/접근 | 우리와의 연결 | 노트 파일 |``
    """
    rows = _parse_table_from_section(section_text)
    notes: list[ReadingNote] = []
    for cells in rows:
        if len(cells) < 5:
            continue
        # Extract source_id from note file path: "read/{id}_note.md"
        note_path = cells[4].strip()
        m = re.search(r"read/(.+?)_note\.md", note_path)
        source_id = m.group(1) if m else f"src_{len(notes)+1}"

        claims_str = cells[1].strip()
        claims = [c.strip() for c in claims_str.split(";") if c.strip()]

        notes.append(
            ReadingNote(
                source_id=source_id,
                source_title=cells[0].strip(),
                passes=[
                    ReadingPass(
                        pass_number=1,
                        claims=claims,
                        method_notes=cells[2].strip(),
                        connection_to_project=cells[3].strip(),
                    )
                ] if any(cells[1:4]) else [],
            )
        )
    return notes


def _parse_write(section_text: str) -> list[SectionState]:
    """Parse the Write table into SectionState objects.

    Columns: ``| 섹션 | 상태 | 비고 |``
    """
    rows = _parse_table_from_section(section_text)
    sections: list[SectionState] = []
    for cells in rows:
        if len(cells) < 3:
            continue
        status = _enum_by_value(SectionStatus, cells[1])
        if status is None:
            # Non-standard status (e.g. "-") → keep as notes
            sections.append(
                SectionState(name=cells[0].strip(), notes=cells[2].strip())
            )
        else:
            sections.append(
                SectionState(
                    name=cells[0].strip(),
                    status=status,
                    notes=cells[2].strip(),
                )
            )
    return sections


def _parse_review(section_text: str) -> list[ReviewRecord]:
    """Parse the Review table into ReviewRecord objects.

    Columns: ``| 날짜 | 리뷰어 | 유형 | 주요 피드백 | 반영 상태 |``
    """
    rows = _parse_table_from_section(section_text)
    records: list[ReviewRecord] = []
    for cells in rows:
        if len(cells) < 5:
            continue
        dt = _parse_date(cells[0])
        records.append(
            ReviewRecord(
                date=dt,
                reviewer=cells[1].strip(),
                type=_enum_by_value(ReviewType, cells[2], ReviewType.SELF_REVIEW),
                summary=cells[3].strip(),
                reflection_status=_enum_by_value(
                    ReviewReflectionStatus, cells[4], ReviewReflectionStatus.PENDING
                ),
            )
        )
    return records


def _parse_claim_evidence(section_text: str) -> list[ClaimEvidence]:
    """Parse the Claim-Evidence table.

    Columns: ``| 주장 | 근거 자료 | 결과/수치 | 위치 |``
    """
    rows = _parse_table_from_section(section_text)
    items: list[ClaimEvidence] = []
    for cells in rows:
        if len(cells) < 4:
            continue
        items.append(
            ClaimEvidence(
                claim=cells[0].strip(),
                evidence_source=cells[1].strip(),
                result=cells[2].strip(),
                location=cells[3].strip(),
            )
        )
    return items


def _parse_open_questions(section_text: str) -> list[str]:
    """Parse bullet list from the 열린 질문 section.

    A single ``-`` (used for "none") returns an empty list.
    """
    questions: list[str] = []
    for line in section_text.split("\n"):
        stripped = line.strip()
        if stripped.startswith("- "):
            text = stripped[2:].strip()
            if text:
                questions.append(text)
        elif stripped == "-":
            continue
    return questions


def _parse_changelog(section_text: str) -> list[ChangeEntry]:
    """Parse the 변경 이력 table.

    Columns: ``| 날짜 | 도메인 | 작업 | 결정 |``
    """
    rows = _parse_table_from_section(section_text)
    entries: list[ChangeEntry] = []
    for cells in rows:
        if len(cells) < 4:
            continue
        dt = _parse_date(cells[0])
        entries.append(
            ChangeEntry(
                date=dt,
                domain=cells[1].strip(),
                action=cells[2].strip(),
                decision=cells[3].strip(),
            )
        )
    return entries


# --- Utilities ---


def _parse_date(text: str) -> date | None:
    """Try to parse a YYYY-MM-DD date, returning None on failure."""
    text = text.strip()
    if not text:
        return None
    m = re.match(r"(\d{4}-\d{2}-\d{2})", text)
    if m:
        try:
            return date.fromisoformat(m.group(1))
        except ValueError:
            return None
    return None


def _split_sections(md_text: str) -> dict[str, str]:
    """Split markdown text by ``## `` headers.

    Returns:
        Dict mapping section header text to section body.
    """
    sections: dict[str, str] = {}
    current_header: str | None = None
    current_lines: list[str] = []

    for line in md_text.split("\n"):
        if line.startswith("## "):
            if current_header is not None:
                sections[current_header] = "\n".join(current_lines)
            current_header = line[3:].strip()
            current_lines = []
        else:
            current_lines.append(line)

    if current_header is not None:
        sections[current_header] = "\n".join(current_lines)

    return sections


def _parse_table_from_section(section_text: str) -> list[list[str]]:
    """Parse all table rows from a section body (skipping header + separator)."""
    rows: list[list[str]] = []
    found_header = False
    skip_separator = False

    for line in section_text.split("\n"):
        stripped = line.strip()
        if not found_header and stripped.startswith("|") and "|" in stripped[1:]:
            found_header = True
            skip_separator = True
            continue
        if skip_separator:
            skip_separator = False
            if stripped.startswith("|") and re.match(r"^\|[\s\-|]+\|$", stripped):
                continue
            # Not a separator — treat as data
        if found_header and stripped.startswith("|") and "|" in stripped[1:]:
            cells = [c.strip() for c in stripped.split("|")[1:-1]]
            if cells and not all(re.match(r"^-+$", c) for c in cells):
                rows.append(cells)
        elif found_header and not stripped.startswith("|") and stripped:
            break

    return rows


def parse_workspace(md_text: str) -> WorkspaceState:
    """Parse a c2_workspace.md markdown string into a WorkspaceState.

    This is the inverse of :func:`render_workspace`.  Some fields are lossy
    (Source.id is regenerated as a slug, truncated ReadingNote fields, etc.)
    because the workspace table format does not store all model fields.

    Args:
        md_text: Markdown content of a c2_workspace.md file.

    Returns:
        WorkspaceState populated from the markdown.
    """
    # Extract project name from "# c2 Workspace - {name}"
    name_match = re.search(r"^#\s+c2\s+Workspace\s+-\s+(.+)$", md_text, re.MULTILINE)
    project_name = name_match.group(1).strip() if name_match else "unknown"

    sections = _split_sections(md_text)

    # Metadata
    meta_text = sections.get("프로젝트 정보", "")
    meta = _parse_metadata(meta_text)

    project_type = _enum_by_value(ProjectType, meta.get("유형", ""), ProjectType.ACADEMIC_PAPER)
    goal = meta.get("목표", "")
    created_at = _parse_date(meta.get("생성일", ""))
    last_session = meta.get("마지막 세션", "")

    # Section dispatch
    sources = _parse_discover(sections.get("Discover (자료 탐색)", ""))
    reading_notes = _parse_read(sections.get("Read (읽기 노트)", ""))
    write_sections = _parse_write(sections.get("Write (작성 상태)", ""))
    reviews = _parse_review(sections.get("Review (리뷰 이력)", ""))
    claim_evidence = _parse_claim_evidence(sections.get("Claim-Evidence 매핑", ""))
    open_questions = _parse_open_questions(sections.get("열린 질문", ""))
    changelog = _parse_changelog(sections.get("변경 이력", ""))

    return WorkspaceState(
        project_name=project_name,
        project_type=project_type,
        goal=goal,
        created_at=created_at,
        last_session=last_session,
        sources=sources,
        reading_notes=reading_notes,
        sections=write_sections,
        reviews=reviews,
        claim_evidence=claim_evidence,
        open_questions=open_questions,
        changelog=changelog,
    )
