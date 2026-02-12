"""OOXML Chart Parser - 차트 XML을 테이블로 변환하는 공통 유틸리티."""

from xml.etree import ElementTree as ET

from c4.c2.parsers.ir_models import create_table

# Chart XML 네임스페이스 (OOXML 호환)
CHART_NAMESPACES = {
    "c": "http://schemas.openxmlformats.org/drawingml/2006/chart",
    "a": "http://schemas.openxmlformats.org/drawingml/2006/main",
}


def parse_chart_xml(xml_content: str) -> dict | None:
    """Chart XML을 테이블 블록으로 변환.

    OOXML 차트 구조:
    - c:ser: 데이터 시리즈 (계열)
    - c:cat: 카테고리 (X축 레이블)
    - c:val: 값 (Y축 데이터)

    Args:
        xml_content: Chart XML 문자열

    Returns:
        테이블 블록 또는 None
    """
    try:
        root = ET.fromstring(xml_content)
    except ET.ParseError:
        return None

    # 모든 시리즈 찾기
    series_list = root.findall(".//c:ser", CHART_NAMESPACES)
    if not series_list:
        return None

    # 카테고리 (X축 레이블) 추출
    categories = _extract_categories(root)
    if not categories:
        return None

    # 시리즈 데이터 추출
    series_data = []
    series_names = []

    for ser in series_list:
        # 시리즈 이름
        tx = ser.find(".//c:tx//c:v", CHART_NAMESPACES)
        series_name = tx.text if tx is not None and tx.text else f"계열 {len(series_names) + 1}"
        series_names.append(series_name)

        # 시리즈 값
        values = _extract_series_values(ser)
        series_data.append(values)

    if not series_data:
        return None

    # 테이블 구성: 첫 열 = 카테고리, 나머지 열 = 시리즈
    header = [""] + series_names
    rows = []

    for i, cat in enumerate(categories):
        row = [cat]
        for series_values in series_data:
            if i < len(series_values):
                row.append(series_values[i])
            else:
                row.append("")
        rows.append(row)

    return create_table(header=header, rows=rows)


def _extract_categories(root) -> list[str]:
    """카테고리 (X축 레이블) 추출."""
    categories = []

    # 문자열 카테고리 확인
    cat_elem = root.find(".//c:cat//c:strCache", CHART_NAMESPACES)
    if cat_elem is not None:
        for pt in cat_elem.findall("c:pt", CHART_NAMESPACES):
            v = pt.find("c:v", CHART_NAMESPACES)
            if v is not None and v.text:
                categories.append(v.text)

    # 숫자 카테고리도 확인
    if not categories:
        cat_num = root.find(".//c:cat//c:numCache", CHART_NAMESPACES)
        if cat_num is not None:
            for pt in cat_num.findall("c:pt", CHART_NAMESPACES):
                v = pt.find("c:v", CHART_NAMESPACES)
                if v is not None and v.text:
                    categories.append(v.text)

    return categories


def _extract_series_values(ser) -> list[str]:
    """시리즈에서 값 추출."""
    values = []
    val_cache = ser.find(".//c:val//c:numCache", CHART_NAMESPACES)
    if val_cache is not None:
        for pt in val_cache.findall("c:pt", CHART_NAMESPACES):
            v = pt.find("c:v", CHART_NAMESPACES)
            if v is not None and v.text:
                values.append(v.text)
            else:
                values.append("")
    return values
