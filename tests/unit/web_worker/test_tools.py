"""Tests for c4.web_worker.tools module."""


from c4.web_worker.tools import (
    TOOLS,
    ToolName,
    get_tool_by_name,
    validate_tool_input,
)


class TestToolDefinitions:
    """Test tool definitions."""

    def test_tools_is_list(self):
        """TOOLS should be a list."""
        assert isinstance(TOOLS, list)

    def test_tools_has_required_tools(self):
        """TOOLS should have all required tools."""
        tool_names = {t["name"] for t in TOOLS}
        expected = {"read_file", "write_file", "run_shell", "search_files", "list_directory"}
        assert expected <= tool_names

    def test_each_tool_has_required_fields(self):
        """Each tool should have name, description, and input_schema."""
        for tool in TOOLS:
            assert "name" in tool, f"Tool missing 'name': {tool}"
            assert "description" in tool, f"Tool missing 'description': {tool}"
            assert "input_schema" in tool, f"Tool missing 'input_schema': {tool}"

    def test_input_schema_is_valid(self):
        """Input schema should have type: object and properties."""
        for tool in TOOLS:
            schema = tool["input_schema"]
            assert schema.get("type") == "object", f"Tool {tool['name']} schema type is not 'object'"
            assert "properties" in schema, f"Tool {tool['name']} schema missing 'properties'"

    def test_read_file_tool(self):
        """read_file tool has correct schema."""
        tool = get_tool_by_name("read_file")
        assert tool is not None
        assert "path" in tool["input_schema"]["properties"]
        assert "path" in tool["input_schema"]["required"]

    def test_write_file_tool(self):
        """write_file tool has correct schema."""
        tool = get_tool_by_name("write_file")
        assert tool is not None
        props = tool["input_schema"]["properties"]
        assert "path" in props
        assert "content" in props
        assert set(tool["input_schema"]["required"]) == {"path", "content"}

    def test_run_shell_tool(self):
        """run_shell tool has correct schema."""
        tool = get_tool_by_name("run_shell")
        assert tool is not None
        props = tool["input_schema"]["properties"]
        assert "command" in props
        assert "timeout" in props
        assert "command" in tool["input_schema"]["required"]

    def test_search_files_tool(self):
        """search_files tool has correct schema."""
        tool = get_tool_by_name("search_files")
        assert tool is not None
        props = tool["input_schema"]["properties"]
        assert "pattern" in props
        assert "search_type" in props
        assert props["search_type"]["enum"] == ["glob", "grep"]
        assert set(tool["input_schema"]["required"]) == {"pattern", "search_type"}

    def test_list_directory_tool(self):
        """list_directory tool has correct schema."""
        tool = get_tool_by_name("list_directory")
        assert tool is not None
        props = tool["input_schema"]["properties"]
        assert "path" in props
        assert "recursive" in props


class TestGetToolByName:
    """Test get_tool_by_name function."""

    def test_get_existing_tool(self):
        """Should return tool for existing name."""
        tool = get_tool_by_name("read_file")
        assert tool is not None
        assert tool["name"] == "read_file"

    def test_get_nonexistent_tool(self):
        """Should return None for nonexistent name."""
        tool = get_tool_by_name("nonexistent_tool")
        assert tool is None

    def test_get_all_tools(self):
        """Should be able to get all tools by name."""
        for tool in TOOLS:
            found = get_tool_by_name(tool["name"])
            assert found is not None
            assert found["name"] == tool["name"]


class TestValidateToolInput:
    """Test validate_tool_input function."""

    def test_valid_read_file_input(self):
        """Valid read_file input should pass."""
        is_valid, error = validate_tool_input("read_file", {"path": "/test.txt"})
        assert is_valid is True
        assert error == ""

    def test_missing_required_field(self):
        """Missing required field should fail."""
        is_valid, error = validate_tool_input("read_file", {})
        assert is_valid is False
        assert "path" in error

    def test_valid_write_file_input(self):
        """Valid write_file input should pass."""
        is_valid, error = validate_tool_input("write_file", {"path": "/test.txt", "content": "hello"})
        assert is_valid is True
        assert error == ""

    def test_write_file_missing_content(self):
        """write_file without content should fail."""
        is_valid, error = validate_tool_input("write_file", {"path": "/test.txt"})
        assert is_valid is False
        assert "content" in error

    def test_valid_run_shell_input(self):
        """Valid run_shell input should pass."""
        is_valid, error = validate_tool_input("run_shell", {"command": "ls"})
        assert is_valid is True

    def test_valid_search_files_input(self):
        """Valid search_files input should pass."""
        is_valid, error = validate_tool_input(
            "search_files",
            {"pattern": "*.py", "search_type": "glob"},
        )
        assert is_valid is True

    def test_search_files_missing_search_type(self):
        """search_files without search_type should fail."""
        is_valid, error = validate_tool_input("search_files", {"pattern": "*.py"})
        assert is_valid is False
        assert "search_type" in error

    def test_valid_list_directory_input(self):
        """list_directory with no required fields should pass."""
        is_valid, error = validate_tool_input("list_directory", {})
        assert is_valid is True

    def test_unknown_tool(self):
        """Unknown tool should fail validation."""
        is_valid, error = validate_tool_input("unknown_tool", {"foo": "bar"})
        assert is_valid is False
        assert "Unknown tool" in error


class TestToolNameConstants:
    """Test ToolName constants."""

    def test_tool_name_values(self):
        """ToolName constants should match tool names."""
        assert ToolName.READ_FILE == "read_file"
        assert ToolName.WRITE_FILE == "write_file"
        assert ToolName.RUN_SHELL == "run_shell"
        assert ToolName.SEARCH_FILES == "search_files"
        assert ToolName.LIST_DIRECTORY == "list_directory"

    def test_tool_names_exist_in_tools(self):
        """All ToolName values should exist in TOOLS."""
        tool_names = {t["name"] for t in TOOLS}
        assert ToolName.READ_FILE in tool_names
        assert ToolName.WRITE_FILE in tool_names
        assert ToolName.RUN_SHELL in tool_names
        assert ToolName.SEARCH_FILES in tool_names
        assert ToolName.LIST_DIRECTORY in tool_names
