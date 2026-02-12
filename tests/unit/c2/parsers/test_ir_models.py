"""IR 모델 테스트."""


from c4.c2.parsers.ir_models import (
    CellStyle,
    Document,
    HeadingBlock,
    MergeInfo,
    ParagraphBlock,
    create_heading,
    create_image,
    create_list,
    create_paragraph,
    create_table,
)


class TestHeadingBlock:
    def test_create(self):
        h = HeadingBlock(level=1, text="제목")
        assert h.type == "heading"
        assert h.level == 1
        assert h.text == "제목"

    def test_helper(self):
        h = create_heading(2, "부제목")
        assert h.level == 2

    def test_level_clamped(self):
        h = create_heading(0, "test")
        assert h.level == 1
        h = create_heading(5, "test")
        assert h.level == 3


class TestParagraphBlock:
    def test_create(self):
        p = ParagraphBlock(text="본문")
        assert p.type == "paragraph"
        assert p.text == "본문"
        assert p.font_size is None
        assert p.is_bold is False

    def test_with_style(self):
        p = create_paragraph("굵은 텍스트", font_size=14.0, is_bold=True)
        assert p.font_size == 14.0
        assert p.is_bold is True


class TestTableBlock:
    def test_create(self):
        t = create_table(
            header=["이름", "나이"],
            rows=[["홍길동", "30"]],
        )
        assert t.type == "table"
        assert len(t.header) == 2
        assert len(t.rows) == 1

    def test_with_merge(self):
        t = create_table(
            header=["A", "B", "C"],
            rows=[["1", "2", "3"]],
            merge_info=[MergeInfo(row=0, col=0, colspan=2)],
        )
        assert t.merge_info is not None
        assert t.merge_info[0].colspan == 2

    def test_with_cell_styles(self):
        styles = [[CellStyle(is_bold=True), CellStyle()]]
        t = create_table(
            header=["H1", "H2"],
            rows=[],
            cell_styles=styles,
        )
        assert t.cell_styles is not None
        assert t.cell_styles[0][0].is_bold is True


class TestImageBlock:
    def test_create(self):
        img = create_image("img001", "image/png", caption="Figure 1")
        assert img.type == "image"
        assert img.image_id == "img001"
        assert img.caption == "Figure 1"


class TestListBlock:
    def test_ordered(self):
        lst = create_list("ordered", ["첫째", "둘째", "셋째"])
        assert lst.type == "list"
        assert lst.list_type == "ordered"
        assert len(lst.items) == 3

    def test_unordered(self):
        lst = create_list("unordered", ["항목"])
        assert lst.list_type == "unordered"


class TestDocument:
    def test_create(self):
        doc = Document(blocks=[
            create_heading(1, "제목"),
            create_paragraph("본문"),
        ])
        assert len(doc.blocks) == 2

    def test_empty(self):
        doc = Document(blocks=[])
        assert len(doc.blocks) == 0

    def test_block_union_type(self):
        """Block 유니온 타입으로 모든 블록 타입 포함 가능."""
        doc = Document(blocks=[
            create_heading(1, "h"),
            create_paragraph("p"),
            create_table(["c"], [["v"]]),
            create_image("id", "image/png"),
            create_list("unordered", ["item"]),
        ])
        assert len(doc.blocks) == 5
