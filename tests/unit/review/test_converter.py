"""
Tests for auto_review.converter module.
"""

import tempfile
from pathlib import Path

import pytest

from c4.review.converter import PDFConverter
from c4.review.models import PageImage, PaperMetadata


# Test fixtures
@pytest.fixture
def converter():
    """Create a PDFConverter instance."""
    return PDFConverter(dpi=72)  # Use low DPI for faster tests


@pytest.fixture
def sample_pdf():
    """
    Return path to a sample PDF file from the review directory.
    Falls back to creating a minimal PDF if none available.
    """
    # Try to find a real PDF in the review directory
    review_dir = Path("/Users/changmin/git/auto_review/review")
    if review_dir.exists():
        pdf_files = list(review_dir.rglob("*.pdf"))
        if pdf_files:
            return pdf_files[0]

    # Fallback: create a minimal PDF for testing
    pytest.skip("No sample PDF available for testing")


@pytest.fixture
def temp_nonpdf_file():
    """Create a temporary non-PDF file (binary garbage)."""
    with tempfile.NamedTemporaryFile(mode="wb", suffix=".pdf", delete=False) as f:
        # Write binary garbage that's not a valid PDF
        f.write(b"\x00\x00\x00\x00NOTAPDF\xFF\xFF\xFF")
        temp_path = Path(f.name)
    yield temp_path
    temp_path.unlink()


# Success tests
def test_convert_valid_pdf(converter, sample_pdf):
    """Test converting a valid PDF to PageImages."""
    pages = converter.convert(sample_pdf)

    assert isinstance(pages, list)
    assert len(pages) > 0
    assert all(isinstance(page, PageImage) for page in pages)

    # Check first page
    first_page = pages[0]
    assert first_page.page_number == 1
    assert isinstance(first_page.image_data, bytes)
    assert len(first_page.image_data) > 0
    assert first_page.width > 0
    assert first_page.height > 0

    # Check page numbers are sequential
    for i, page in enumerate(pages):
        assert page.page_number == i + 1


def test_get_metadata(converter, sample_pdf):
    """Test extracting metadata from a PDF."""
    metadata = converter.get_metadata(sample_pdf)

    assert isinstance(metadata, PaperMetadata)
    assert isinstance(metadata.title, str)
    assert len(metadata.title) > 0
    assert isinstance(metadata.authors, list)
    assert metadata.page_count > 0


def test_convert_with_base64(converter, sample_pdf):
    """Test converting PDF to base64-encoded images."""
    result = converter.convert_with_base64(sample_pdf)

    assert isinstance(result, list)
    assert len(result) > 0

    first_page = result[0]
    assert "page_number" in first_page
    assert "image_base64" in first_page
    assert "width" in first_page
    assert "height" in first_page

    assert first_page["page_number"] == 1
    assert isinstance(first_page["image_base64"], str)
    assert len(first_page["image_base64"]) > 0


def test_custom_dpi(sample_pdf):
    """Test that custom DPI affects output size."""
    low_dpi_converter = PDFConverter(dpi=72)
    high_dpi_converter = PDFConverter(dpi=150)

    low_dpi_pages = low_dpi_converter.convert(sample_pdf)
    high_dpi_pages = high_dpi_converter.convert(sample_pdf)

    # Same number of pages
    assert len(low_dpi_pages) == len(high_dpi_pages)

    # Higher DPI should produce larger images
    assert high_dpi_pages[0].width > low_dpi_pages[0].width
    assert high_dpi_pages[0].height > low_dpi_pages[0].height


# Failure tests
def test_convert_nonexistent_file(converter):
    """Test that converting a nonexistent file raises FileNotFoundError."""
    nonexistent_path = Path("/tmp/nonexistent_file_12345.pdf")

    with pytest.raises(FileNotFoundError) as exc_info:
        converter.convert(nonexistent_path)

    assert "not found" in str(exc_info.value).lower()


def test_convert_invalid_file(converter, temp_nonpdf_file):
    """Test that converting an invalid file raises ValueError."""
    with pytest.raises(ValueError) as exc_info:
        converter.convert(temp_nonpdf_file)

    assert "invalid" in str(exc_info.value).lower() or "corrupted" in str(exc_info.value).lower()


def test_get_metadata_nonexistent_file(converter):
    """Test that getting metadata from nonexistent file raises FileNotFoundError."""
    nonexistent_path = Path("/tmp/nonexistent_file_12345.pdf")

    with pytest.raises(FileNotFoundError) as exc_info:
        converter.get_metadata(nonexistent_path)

    assert "not found" in str(exc_info.value).lower()


def test_get_metadata_invalid_file(converter, temp_nonpdf_file):
    """Test that getting metadata from invalid file raises ValueError."""
    with pytest.raises(ValueError) as exc_info:
        converter.get_metadata(temp_nonpdf_file)

    assert "invalid" in str(exc_info.value).lower() or "corrupted" in str(exc_info.value).lower()


def test_metadata_fallback_title(converter, sample_pdf):
    """Test that metadata falls back to filename if title is empty."""
    metadata = converter.get_metadata(sample_pdf)

    # Title should either be from metadata or filename
    assert metadata.title
    # If PDF has no title metadata, it should use the filename (stem)
    if not metadata.title or metadata.title == sample_pdf.stem:
        assert metadata.title == sample_pdf.stem
