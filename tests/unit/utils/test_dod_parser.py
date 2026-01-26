"""Tests for DoD parser utility."""


from c4.models.ddd import DoDItem
from c4.utils.dod_parser import (
    format_dod,
    generate_standard_dod,
    get_completion_stats,
    get_overall_completion,
    parse_dod,
    update_dod_item,
    validate_dod_requirements,
)


class TestParseDod:
    """Tests for parse_dod function."""

    def test_parse_markdown_checklist(self):
        """Parse markdown checklist format."""
        dod = """
        - [ ] UserService.register() 구현
        - [x] test_register_success 통과
        - [ ] lint 통과
        """
        items = parse_dod(dod)

        assert len(items) == 3
        assert items[0].text == "UserService.register() 구현"
        assert items[0].completed is False
        assert items[0].category == "impl"

        assert items[1].text == "test_register_success 통과"
        assert items[1].completed is True
        assert items[1].category == "test"

        assert items[2].text == "lint 통과"
        assert items[2].completed is False
        assert items[2].category == "gate"

    def test_parse_numbered_list(self):
        """Parse numbered list format."""
        dod = """
        1. Create user service
        2. Add unit tests
        3. Run lint
        """
        items = parse_dod(dod)

        assert len(items) == 3
        assert items[0].text == "Create user service"
        assert items[1].text == "Add unit tests"
        assert items[2].text == "Run lint"

    def test_parse_bullet_points(self):
        """Parse bullet point format."""
        dod = """
        • 기능 구현
        * 테스트 작성
        - 검증 통과
        """
        items = parse_dod(dod)

        assert len(items) == 3

    def test_parse_mixed_format(self):
        """Parse mixed format."""
        dod = """
        - [ ] First implementation item
        1. Second item
        • Third item
        """
        items = parse_dod(dod)

        assert len(items) == 3

    def test_parse_empty_dod(self):
        """Parse empty DoD."""
        items = parse_dod("")
        assert len(items) == 0

    def test_parse_dod_with_empty_lines(self):
        """Parse DoD with empty lines."""
        dod = """
        - [ ] Item 1

        - [ ] Item 2

        """
        items = parse_dod(dod)
        assert len(items) == 2

    def test_classify_test_items(self):
        """Test items are classified correctly."""
        dod = """
        - [ ] test_success 통과
        - [ ] pytest 실행
        - [ ] coverage 확인
        """
        items = parse_dod(dod)

        assert all(item.category == "test" for item in items)

    def test_classify_gate_items(self):
        """Gate items are classified correctly."""
        dod = """
        - [ ] lint 통과
        - [ ] ruff check 실행
        - [ ] mypy 타입 검사
        """
        items = parse_dod(dod)

        assert all(item.category == "gate" for item in items)

    def test_classify_review_items(self):
        """Review items are classified correctly."""
        dod = """
        - [ ] 코드 리뷰 완료
        - [ ] 검토 승인
        """
        items = parse_dod(dod)

        assert all(item.category == "review" for item in items)


class TestFormatDod:
    """Tests for format_dod function."""

    def test_format_dod_items(self):
        """Format DoD items back to markdown."""
        items = [
            DoDItem(text="Item 1", completed=False, category="impl"),
            DoDItem(text="Item 2", completed=True, category="test"),
            DoDItem(text="Item 3", completed=False, category="gate"),
        ]

        formatted = format_dod(items)

        assert "- [ ] Item 1" in formatted
        assert "- [x] Item 2" in formatted
        assert "- [ ] Item 3" in formatted

    def test_format_empty_list(self):
        """Format empty list."""
        formatted = format_dod([])
        assert formatted == ""


class TestUpdateDodItem:
    """Tests for update_dod_item function."""

    def test_update_single_item(self):
        """Update single matching item."""
        items = [
            DoDItem(text="Item 1", completed=False),
            DoDItem(text="Item 2", completed=False),
        ]

        updated = update_dod_item(items, "Item 1", completed=True)

        assert updated[0].completed is True
        assert updated[1].completed is False

    def test_update_multiple_matches(self):
        """Update multiple matching items."""
        items = [
            DoDItem(text="Test item A", completed=False),
            DoDItem(text="Test item B", completed=False),
            DoDItem(text="Other item", completed=False),
        ]

        updated = update_dod_item(items, "Test", completed=True)

        assert updated[0].completed is True
        assert updated[1].completed is True
        assert updated[2].completed is False

    def test_update_case_insensitive(self):
        """Update is case insensitive."""
        items = [DoDItem(text="LINT 통과", completed=False)]
        updated = update_dod_item(items, "lint", completed=True)
        assert updated[0].completed is True


class TestGetCompletionStats:
    """Tests for get_completion_stats function."""

    def test_stats_by_category(self):
        """Get stats grouped by category."""
        items = [
            DoDItem(text="impl 1", completed=True, category="impl"),
            DoDItem(text="impl 2", completed=False, category="impl"),
            DoDItem(text="test 1", completed=True, category="test"),
            DoDItem(text="gate 1", completed=True, category="gate"),
        ]

        stats = get_completion_stats(items)

        assert stats["impl"] == 1  # 1 completed out of 2
        assert stats["test"] == 1
        assert stats["gate"] == 1


class TestGetOverallCompletion:
    """Tests for get_overall_completion function."""

    def test_overall_completion_partial(self):
        """Get overall completion percentage."""
        items = [
            DoDItem(text="item 1", completed=True),
            DoDItem(text="item 2", completed=True),
            DoDItem(text="item 3", completed=False),
            DoDItem(text="item 4", completed=False),
        ]

        completed, total, percentage = get_overall_completion(items)

        assert completed == 2
        assert total == 4
        assert percentage == 50.0

    def test_overall_completion_empty(self):
        """Get completion for empty list."""
        completed, total, percentage = get_overall_completion([])
        assert completed == 0
        assert total == 0
        assert percentage == 0.0

    def test_overall_completion_all_done(self):
        """Get completion when all done."""
        items = [
            DoDItem(text="item 1", completed=True),
            DoDItem(text="item 2", completed=True),
        ]

        _, _, percentage = get_overall_completion(items)
        assert percentage == 100.0


class TestValidateDodRequirements:
    """Tests for validate_dod_requirements function."""

    def test_valid_dod(self):
        """Valid DoD passes validation."""
        items = [
            DoDItem(text="구현 완료", category="impl"),
            DoDItem(text="test_success 통과", category="test"),
            DoDItem(text="test_failure 확인", category="test"),
            DoDItem(text="test_boundary 추가", category="test"),
            DoDItem(text="lint 통과", category="gate"),
        ]

        errors = validate_dod_requirements(items)
        assert len(errors) == 0

    def test_missing_impl(self):
        """DoD missing implementation items."""
        items = [
            DoDItem(text="test_success", category="test"),
            DoDItem(text="lint 통과", category="gate"),
        ]

        errors = validate_dod_requirements(items)
        assert any("implementation" in e.lower() for e in errors)

    def test_missing_test(self):
        """DoD missing test items."""
        items = [
            DoDItem(text="구현 완료", category="impl"),
            DoDItem(text="lint 통과", category="gate"),
        ]

        errors = validate_dod_requirements(items)
        assert any("test" in e.lower() for e in errors)

    def test_missing_gate(self):
        """DoD missing gate items."""
        items = [
            DoDItem(text="구현 완료", category="impl"),
            DoDItem(text="test_success", category="test"),
        ]

        errors = validate_dod_requirements(items)
        assert any("gate" in e.lower() for e in errors)


class TestGenerateStandardDod:
    """Tests for generate_standard_dod function."""

    def test_generate_dod(self):
        """Generate standard DoD string."""
        dod = generate_standard_dod(
            impl_items=["UserService.register() 구현"],
            test_items=["test_register_success 통과", "test_register_failure 확인"],
        )

        assert "UserService.register()" in dod
        assert "test_register_success" in dod
        assert "lint 통과" in dod  # Default gate

    def test_generate_dod_custom_gates(self):
        """Generate DoD with custom gates."""
        dod = generate_standard_dod(
            impl_items=["구현"],
            test_items=["테스트"],
            gates=["ruff check", "mypy"],
        )

        assert "ruff check" in dod
        assert "mypy" in dod
        assert "lint 통과" not in dod  # Default not included
