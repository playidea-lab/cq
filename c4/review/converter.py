"""
PDF to image converter using pypdfium2.
"""

import base64
from pathlib import Path

import pypdfium2 as pdfium

from c4.review.models import PageImage, PaperMetadata


class PDFConverter:
    """Converts PDF files to page images and extracts metadata."""

    def __init__(self, dpi: int = 150):
        """
        Initialize the PDF converter.

        Args:
            dpi: Resolution for rendering pages (default: 150)
        """
        self.dpi = dpi

    def convert(self, pdf_path: Path) -> list[PageImage]:
        """
        Convert PDF pages to PNG images.

        Args:
            pdf_path: Path to the PDF file

        Returns:
            List of PageImage objects, one per page

        Raises:
            FileNotFoundError: If the PDF file does not exist
            ValueError: If the file is not a valid PDF or is corrupted
        """
        if not pdf_path.exists():
            raise FileNotFoundError(f"PDF file not found: {pdf_path}")

        try:
            doc = pdfium.PdfDocument(pdf_path)
        except Exception as e:
            raise ValueError(f"Invalid or corrupted PDF file: {e}")

        page_count = len(doc)
        if page_count == 0:
            doc.close()
            raise ValueError("PDF file has no pages")

        pages = []
        try:
            scale = self.dpi / 72.0
            for page_num in range(page_count):
                page = doc[page_num]

                # Render page to bitmap
                bitmap = page.render(scale=scale)
                pil_image = bitmap.to_pil()

                # Convert to PNG bytes
                import io

                buf = io.BytesIO()
                pil_image.save(buf, format="PNG")
                png_data = buf.getvalue()

                pages.append(
                    PageImage(
                        page_number=page_num + 1,  # 1-indexed
                        image_data=png_data,
                        width=pil_image.width,
                        height=pil_image.height,
                    )
                )
        finally:
            doc.close()

        return pages

    def get_metadata(self, pdf_path: Path) -> PaperMetadata:
        """
        Extract metadata from PDF file.

        Args:
            pdf_path: Path to the PDF file

        Returns:
            PaperMetadata object

        Raises:
            FileNotFoundError: If the PDF file does not exist
            ValueError: If the file is not a valid PDF
        """
        if not pdf_path.exists():
            raise FileNotFoundError(f"PDF file not found: {pdf_path}")

        try:
            doc = pdfium.PdfDocument(pdf_path)
        except Exception as e:
            raise ValueError(f"Invalid or corrupted PDF file: {e}")

        try:
            metadata = doc.get_metadata_dict()
            title = metadata.get("Title", "").strip()
            if not title:
                title = pdf_path.stem

            author = metadata.get("Author", "").strip()
            authors = [author] if author else []

            return PaperMetadata(
                title=title,
                authors=authors,
                page_count=len(doc),
            )
        finally:
            doc.close()

    def convert_with_base64(self, pdf_path: Path) -> list[dict]:
        """
        Convert PDF pages to base64-encoded PNG images.

        Args:
            pdf_path: Path to the PDF file

        Returns:
            List of dicts with page_number, image_base64, width, height

        Raises:
            FileNotFoundError: If the PDF file does not exist
            ValueError: If the file is not a valid PDF
        """
        pages = self.convert(pdf_path)
        return [
            {
                "page_number": page.page_number,
                "image_base64": base64.b64encode(page.image_data).decode("utf-8"),
                "width": page.width,
                "height": page.height,
            }
            for page in pages
        ]
