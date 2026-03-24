"""PDF Parser - OpenDataLoader PDF 기반."""

import json
import logging
import tempfile
from pathlib import Path

import opendataloader_pdf

from c4.c2.parsers.base import BaseParser, ImageData, ParseResult
from c4.c2.parsers.ir_models import (
    Block,
    Document,
    MergeInfo,
    create_heading,
    create_image,
    create_list,
    create_paragraph,
    create_table,
)
from c4.c2.parsers.utils.image import generate_image_id, get_mime_from_extension

logger = logging.getLogger(__name__)


class PdfParser(BaseParser):
    """PDF 문서 파서.

    OpenDataLoader PDF v2.0 기반:
    - XY-Cut++ 읽기 순서 알고리즘
    - 테이블 자동 감지 (bordered + borderless)
    - 이미지 추출 (좌표 포함)
    - OCR 지원 (80+ 언어)
    """

    @property
    def supported_extensions(self) -> list[str]:
        return [".pdf"]

    def parse(self, file_path: Path) -> Document:
        """PDF 파일을 IR로 변환."""
        result = self.parse_with_images(file_path)
        return result.document

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """PDF 파일을 IR과 이미지로 변환."""
        with tempfile.TemporaryDirectory() as tmpdir:
            out_dir = Path(tmpdir) / "output"
            img_dir = out_dir / "images"
            out_dir.mkdir()
            img_dir.mkdir()

            try:
                opendataloader_pdf.convert(
                    input_path=[str(file_path)],
                    output_dir=str(out_dir),
                    format="json",
                    image_output="external",
                    image_format="png",
                    image_dir=str(img_dir),
                    quiet=True,
                )
            except Exception as e:
                logger.error("OpenDataLoader PDF failed for %s: %s", file_path, e)
                return ParseResult(document=Document(blocks=[]))

            # Find JSON output
            json_files = list(out_dir.glob("*.json"))
            if not json_files:
                logger.warning("No JSON output from OpenDataLoader for %s", file_path)
                return ParseResult(document=Document(blocks=[]))

            with open(json_files[0], encoding="utf-8") as f:
                odl_data = json.load(f)

            # Convert ODL JSON to IR
            blocks: list[Block] = []
            extracted_images: list[ImageData] = []
            image_index = 0

            for kid in odl_data.get("kids", []):
                converted, new_images, image_index = self._convert_element(
                    kid, img_dir, image_index
                )
                blocks.extend(converted)
                extracted_images.extend(new_images)

            return ParseResult(
                document=Document(blocks=blocks),
                images=extracted_images,
            )

    def _convert_element(
        self, element: dict, img_dir: Path, image_index: int
    ) -> tuple[list[Block], list[ImageData], int]:
        """ODL JSON 요소를 IR 블록으로 변환."""
        elem_type = element.get("type", "")
        blocks: list[Block] = []
        images: list[ImageData] = []

        if elem_type == "heading":
            level = element.get("heading level", 1)
            if isinstance(level, str):
                level = 1
            text = element.get("content", "").strip()
            if text:
                blocks.append(create_heading(level=level, text=text))

        elif elem_type == "paragraph":
            text = element.get("content", "").strip()
            font_size = element.get("font size")
            if text:
                blocks.append(create_paragraph(text=text, font_size=font_size))

        elif elem_type == "list":
            items = []
            for item in element.get("list items", []):
                content = item.get("content", "").strip()
                if content:
                    items.append(content)
            if items:
                numbering = element.get("numbering style", "")
                list_type = "ordered" if "number" in numbering.lower() else "unordered"
                blocks.append(create_list(list_type=list_type, items=items))

        elif elem_type == "table":
            table_block = self._convert_table(element)
            if table_block:
                blocks.append(table_block)

        elif elem_type == "image":
            img_block, img_data, image_index = self._convert_image(
                element, img_dir, image_index
            )
            if img_block:
                blocks.append(img_block)
            if img_data:
                images.append(img_data)

        elif elem_type == "figure":
            # Figure may contain nested kids (image + caption)
            caption = None
            for kid in element.get("kids", []):
                if kid.get("type") == "caption":
                    caption = kid.get("content", "").strip()
                elif kid.get("type") == "image":
                    img_block, img_data, image_index = self._convert_image(
                        kid, img_dir, image_index, caption=caption
                    )
                    if img_block:
                        blocks.append(img_block)
                    if img_data:
                        images.append(img_data)
                else:
                    converted, new_images, image_index = self._convert_element(
                        kid, img_dir, image_index
                    )
                    blocks.extend(converted)
                    images.extend(new_images)

        else:
            # Unknown type — check for nested kids
            for kid in element.get("kids", []):
                converted, new_images, image_index = self._convert_element(
                    kid, img_dir, image_index
                )
                blocks.extend(converted)
                images.extend(new_images)

        return blocks, images, image_index

    def _convert_table(self, element: dict) -> Block | None:
        """ODL 테이블을 TableBlock으로 변환."""
        rows_data = element.get("rows", [])
        if not rows_data:
            return None

        all_rows: list[list[str]] = []
        merge_info: list[MergeInfo] = []

        for row in rows_data:
            cells = row.get("cells", [])
            row_texts: list[str] = []
            for cell in cells:
                # Cell content from nested kids
                cell_text = self._extract_cell_text(cell)
                row_texts.append(cell_text)

                # Merge info
                row_span = cell.get("row span", 1)
                col_span = cell.get("column span", 1)
                if row_span > 1 or col_span > 1:
                    row_num = cell.get("row number", 1) - 1
                    col_num = cell.get("column number", 1) - 1
                    merge_info.append(
                        MergeInfo(
                            row=row_num,
                            col=col_num,
                            rowspan=row_span,
                            colspan=col_span,
                        )
                    )

            all_rows.append(row_texts)

        if not all_rows:
            return None

        # First row as header
        header = all_rows[0]
        body = all_rows[1:] if len(all_rows) > 1 else []

        return create_table(
            header=header,
            rows=body,
            merge_info=merge_info if merge_info else None,
        )

    def _extract_cell_text(self, cell: dict) -> str:
        """테이블 셀에서 텍스트 추출."""
        parts = []
        for kid in cell.get("kids", []):
            content = kid.get("content", "")
            if content:
                parts.append(content.strip())
            # Recurse into nested kids
            for nested in kid.get("kids", []):
                nested_content = nested.get("content", "")
                if nested_content:
                    parts.append(nested_content.strip())
        return " ".join(parts) if parts else ""

    def _convert_image(
        self,
        element: dict,
        img_dir: Path,
        image_index: int,
        caption: str | None = None,
    ) -> tuple[Block | None, ImageData | None, int]:
        """ODL 이미지를 ImageBlock + ImageData로 변환."""
        # Find the image file
        file_name = element.get("file name", "")
        if file_name:
            img_path = img_dir / file_name
        else:
            # Try to find by index pattern
            img_path = img_dir / f"imageFile{image_index + 1}.png"

        if not img_path.exists():
            # Try any image file matching
            candidates = list(img_dir.glob(f"imageFile{image_index + 1}.*"))
            if candidates:
                img_path = candidates[0]
            else:
                return None, None, image_index

        try:
            img_data_bytes = img_path.read_bytes()
            if len(img_data_bytes) < 100:  # Too small, skip
                return None, None, image_index

            ext = img_path.suffix.lower().lstrip(".")
            mime_type, ext_with_dot = get_mime_from_extension(ext)
            image_id = generate_image_id(img_data_bytes, image_index)
            image_id_with_ext = f"{image_id}{ext_with_dot}"

            img_block = create_image(
                image_id=image_id_with_ext,
                mime_type=mime_type,
                caption=caption,
            )
            img_data = ImageData(
                image_id=image_id_with_ext,
                data=img_data_bytes,
                mime_type=mime_type,
            )

            return img_block, img_data, image_index + 1

        except Exception as e:
            logger.debug("Failed to extract image %s: %s", img_path, e)
            return None, None, image_index
