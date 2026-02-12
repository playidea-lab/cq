"""TableParser - HWP 5.0 표 개체 파서.

HWP 5.0 스펙에 따른 표(Table) 개체 파싱:
- HWPTAG_TABLE: 표 속성 (행/열 수, 셀 간격, 여백)
- HWPTAG_LIST_HEADER: 셀 리스트 헤더
- 셀 속성: 위치, 병합, 크기 정보

참조:
- HWP 5.0 스펙 4.3.9.1절: 표 개체
"""

import struct
from dataclasses import dataclass, field

from .record_parser import HwpTagId, Record, RecordParser


def decode_special_chars(text: str) -> str:
    """특수 문자를 처리합니다.

    PUA(Private Use Area) 문자와 Wingdings 문자를 표준 유니코드로 변환합니다.

    Args:
        text: 입력 텍스트

    Returns:
        특수 문자가 처리된 텍스트
    """
    result = []
    for char in text:
        code = ord(char)

        # HWP 전용 코드 영역 (PUA: Private Use Area)
        if 0xE000 <= code <= 0xF8FF:
            # 원문자 (㉠-㉻)
            if 0xE000 <= code <= 0xE01B:
                result.append(chr(0x3260 + (code - 0xE000)))
            # 로마 숫자 (Ⅰ-Ⅻ)
            elif 0xE020 <= code <= 0xE02B:
                result.append(chr(0x2160 + (code - 0xE020)))
            # 화살표 (→, ←, ↑, ↓ 등)
            elif 0xE030 <= code <= 0xE03F:
                arrows = "→←↑↓↔↕↖↗↘↙⇒⇐⇑⇓⇔⇕"
                idx = code - 0xE030
                if idx < len(arrows):
                    result.append(arrows[idx])
                else:
                    result.append(char)
            # Wingdings 화살표 (U+F0E8 = → 등)
            elif code == 0xF0E8:
                result.append("→")
            elif code == 0xF0E7:
                result.append("←")
            elif code == 0xF0E9:
                result.append("↑")
            elif code == 0xF0EA:
                result.append("↓")
            # 체크박스 문자 (Wingdings)
            elif code == 0xF06F:  # 빈 체크박스
                result.append("☐")
            elif code == 0xF0FE:  # 체크된 박스
                result.append("☑")
            elif code == 0xF0FD:  # X 체크박스
                result.append("☒")
            else:
                # 알 수 없는 PUA는 그대로 유지
                result.append(char)
        else:
            result.append(char)

    return "".join(result)

# HWPUNIT 변환 상수
HWPUNIT_TO_PT = 72 / 7200  # 1/7200인치 → pt


def make_ctrl_id(a: str, b: str, c: str, d: str) -> int:
    """4글자 컨트롤 ID를 생성합니다.

    MAKE_4CHID(a, b, c, d) = (a << 24) | (b << 16) | (c << 8) | d
    """
    return (ord(a) << 24) | (ord(b) << 16) | (ord(c) << 8) | ord(d)


# 컨트롤 ID 상수
CTRL_ID_TABLE = make_ctrl_id("t", "b", "l", " ")  # 표
CTRL_ID_GSO = make_ctrl_id("g", "s", "o", " ")  # 그리기 개체


@dataclass
class TableMargin:
    """표 또는 셀의 여백 정보."""

    left: float = 0.0  # pt
    right: float = 0.0  # pt
    top: float = 0.0  # pt
    bottom: float = 0.0  # pt


@dataclass
class Cell:
    """표 셀 정보."""

    col: int = 0  # 열 위치 (0부터 시작)
    row: int = 0  # 행 위치 (0부터 시작)
    colspan: int = 1  # 열 병합 개수
    rowspan: int = 1  # 행 병합 개수
    width: float = 0.0  # 셀 폭 (pt)
    height: float = 0.0  # 셀 높이 (pt)
    margin: TableMargin = field(default_factory=TableMargin)
    border_fill_id: int = 0
    paragraphs: list = field(default_factory=list)  # 셀 내 문단들
    first_char_shape_id: int = -1  # 첫 문단의 글자 모양 ID (-1=미설정)
    image_bin_ids: list[int] = field(default_factory=list)  # 셀 내 이미지 bin_id들

    @property
    def is_merged(self) -> bool:
        """셀이 병합되었는지 확인합니다."""
        return self.colspan > 1 or self.rowspan > 1

    @property
    def end_col(self) -> int:
        """셀의 끝 열 위치 (병합 고려)."""
        return self.col + self.colspan - 1

    @property
    def end_row(self) -> int:
        """셀의 끝 행 위치 (병합 고려)."""
        return self.row + self.rowspan - 1


@dataclass
class Table:
    """표 정보."""

    row_count: int = 0  # 행 수
    col_count: int = 0  # 열 수
    cell_spacing: float = 0.0  # 셀 간격 (pt)
    margin: TableMargin = field(default_factory=TableMargin)
    border_fill_id: int = 0
    cells: list[Cell] = field(default_factory=list)
    row_heights: list[float] = field(default_factory=list)  # 각 행의 높이 (pt)

    # 표 속성 비트
    page_break: int = 0  # 쪽 경계에서 나눔 (0: 나누지 않음, 1: 셀 단위로 나눔)
    repeat_header: bool = False  # 제목 줄 자동 반복 여부

    # 중첩 테이블 정보
    level: int = 0  # 중첩 레벨 (0: 최상위)
    parent_cell: Cell | None = None  # 부모 셀 (중첩 테이블인 경우)

    # 레코드 순서 (문서 내 위치)
    record_index: int = 0

    # Y 좌표 (정렬용, 페이지 내 수직 위치)
    y_offset: int = 0

    # 시작 레코드의 level (중첩 테이블 감지용)
    start_level: int = 0

    def get_cell(self, row: int, col: int) -> Cell | None:
        """특정 위치의 셀을 반환합니다."""
        for cell in self.cells:
            if cell.row == row and cell.col == col:
                return cell
            # 병합된 셀 범위 내에 있는지 확인
            if (
                cell.row <= row <= cell.end_row
                and cell.col <= col <= cell.end_col
            ):
                return cell
        return None

    def get_row(self, row_idx: int) -> list[Cell]:
        """특정 행의 셀들을 반환합니다."""
        return [cell for cell in self.cells if cell.row == row_idx]


class TableParser:
    """HWP 표 개체 파서.

    사용법:
        parser = TableParser()
        tables = parser.parse(section_data)

        # 또는 레코드 리스트에서 직접 파싱
        records = RecordParser().parse_records(data)
        tables = parser.parse_from_records(records)
    """

    def __init__(self) -> None:
        """TableParser를 초기화합니다."""
        self._record_parser = RecordParser()
        self._cell_stack: list[Cell] = []  # 중첩 테이블용 셀 스택

    def parse(self, data: bytes) -> list[Table]:
        """BodyText 섹션 데이터에서 표를 파싱합니다.

        Args:
            data: BodyText/Section* 스트림 데이터

        Returns:
            Table 리스트
        """
        records = self._record_parser.parse_records(data, decompress=True)
        return self.parse_from_records(records)

    def parse_from_records(self, records: list[Record]) -> list[Table]:
        """레코드 리스트에서 표를 추출합니다.

        Args:
            records: Record 리스트

        Returns:
            Table 리스트
        """
        tables: list[Table] = []
        i = 0
        table_stack: list[Table] = []  # 중첩 테이블 스택
        self._cell_stack = []  # 셀 스택 초기화

        while i < len(records):
            record = records[i]

            # CTRL_HEADER에서 컨트롤 ID 확인
            if record.tag_id == HwpTagId.CTRL_HEADER:
                ctrl_id = self._parse_ctrl_id(record)
                if ctrl_id == CTRL_ID_TABLE:
                    # 테이블 시작
                    level = len(table_stack)
                    table = Table(level=level)
                    table.record_index = i  # 레코드 순서 저장
                    table.start_level = record.level  # 시작 레코드의 level 저장 (중첩 감지용)
                    # y_offset 파싱 (offset 8에 위치)
                    if len(record.data) >= 12:
                        table.y_offset = struct.unpack_from("<i", record.data, 8)[0]
                    # 현재 셀이 있으면 부모 셀로 설정 (중첩 테이블)
                    if self._cell_stack:
                        table.parent_cell = self._cell_stack[-1]
                    table_stack.append(table)

            # HWPTAG_TABLE: 표 속성 파싱
            elif record.tag_id == HwpTagId.TABLE and table_stack:
                current_table = table_stack[-1]
                self._parse_table_properties(record, current_table)

            # HWPTAG_LIST_HEADER: 셀 리스트 헤더
            elif record.tag_id == HwpTagId.LIST_HEADER and table_stack:
                current_table = table_stack[-1]
                cell = self._parse_cell_list_header(record)
                if cell:
                    current_table.cells.append(cell)
                    # 셀 스택에 push (중첩 테이블 진입 시 복원용)
                    self._cell_stack.append(cell)

            # HWPTAG_PARA_TEXT: 셀 내부 문단 텍스트
            # level <= 1이면 테이블 외부 문단이므로 무시
            elif record.tag_id == HwpTagId.PARA_TEXT and table_stack and self._cell_stack:
                if record.level >= 2:  # 테이블 셀 내부는 level >= 2
                    text = self._parse_para_text(record)
                    if text:
                        self._cell_stack[-1].paragraphs.append(text)

            # HWPTAG_PARA_CHAR_SHAPE: 셀 내부 문단의 글자 모양
            # 첫 번째 char_shape_id만 저장 (볼드 등 스타일 확인용)
            elif record.tag_id == HwpTagId.PARA_CHAR_SHAPE and table_stack and self._cell_stack:
                current_cell = self._cell_stack[-1]
                if record.level >= 2 and current_cell.first_char_shape_id == -1:
                    char_shape_id = self._parse_first_char_shape_id(record)
                    if char_shape_id >= 0:  # 0도 유효한 CharShape ID
                        current_cell.first_char_shape_id = char_shape_id

            # GSO 내 이미지 감지: CTRL_HEADER가 GSO이고 셀 내부인 경우
            elif record.tag_id == HwpTagId.CTRL_HEADER and table_stack and self._cell_stack:
                ctrl_id = self._parse_ctrl_id(record)
                if ctrl_id == CTRL_ID_GSO and record.level >= 2:
                    # 셀 내부 GSO - 이후 SHAPE_COMPONENT_PICTURE에서 bin_id 수집
                    pass

            # SHAPE_COMPONENT_PICTURE: 셀 내 이미지 bin_id 추출
            elif record.tag_id == HwpTagId.SHAPE_COMPONENT_PICTURE and table_stack and self._cell_stack:
                if record.level >= 3:  # GSO 내부 (셀보다 깊은 레벨)
                    bin_id = self._parse_picture_bin_id(record)
                    if bin_id is not None:
                        self._cell_stack[-1].image_bin_ids.append(bin_id)

            # 테이블 종료 감지 (레벨 기반)
            if table_stack and self._is_table_end(records, i, table_stack[-1]):
                completed_table = table_stack.pop()
                # 완료된 테이블의 셀들을 스택에서 제거
                cells_to_remove = len(completed_table.cells)
                for _ in range(cells_to_remove):
                    if self._cell_stack:
                        self._cell_stack.pop()

                if not table_stack:
                    # 최상위 테이블 완료
                    tables.append(completed_table)
                else:
                    # 중첩 테이블은 부모 셀에 추가
                    if completed_table.parent_cell:
                        completed_table.parent_cell.paragraphs.append(completed_table)

            i += 1

        # 남은 테이블 처리
        while table_stack:
            tables.append(table_stack.pop())

        return tables

    def _parse_ctrl_id(self, record: Record) -> int:
        """CTRL_HEADER에서 컨트롤 ID를 추출합니다."""
        if len(record.data) < 4:
            return 0
        return struct.unpack_from("<I", record.data, 0)[0]

    def _parse_table_properties(self, record: Record, table: Table) -> None:
        """HWPTAG_TABLE 레코드를 파싱하여 표 속성을 설정합니다.

        표 75 표 개체 속성:
        - UINT32: 속성
        - UINT16: RowCount
        - UINT16: nCols
        - HWPUNIT16: CellSpacing
        - BYTE stream[8]: 안쪽 여백 정보
        - UINT16[RowCount]: Row Size 배열
        - UINT16: Border Fill ID
        """
        data = record.data
        if len(data) < 14:  # 최소 크기
            return

        offset = 0

        # 속성 (UINT32)
        props = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 속성 비트 파싱
        table.page_break = props & 0x03  # bit 0-1
        table.repeat_header = bool((props >> 2) & 0x01)  # bit 2

        # RowCount (UINT16)
        table.row_count = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # nCols (UINT16)
        table.col_count = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # CellSpacing (HWPUNIT16)
        cell_spacing_raw = struct.unpack_from("<H", data, offset)[0]
        table.cell_spacing = cell_spacing_raw * HWPUNIT_TO_PT
        offset += 2

        # 안쪽 여백 정보 (8바이트)
        if len(data) >= offset + 8:
            table.margin = self._parse_margin(data, offset)
            offset += 8

        # Row Size 배열 (UINT16 × RowCount)
        if len(data) >= offset + 2 * table.row_count:
            row_heights: list[float] = []
            for _ in range(table.row_count):
                row_size = struct.unpack_from("<H", data, offset)[0]
                row_heights.append(row_size * HWPUNIT_TO_PT)
                offset += 2
            table.row_heights = row_heights

        # Border Fill ID (UINT16)
        if len(data) >= offset + 2:
            table.border_fill_id = struct.unpack_from("<H", data, offset)[0]

    def _parse_margin(self, data: bytes, offset: int) -> TableMargin:
        """안쪽 여백 정보를 파싱합니다.

        표 77 안쪽 여백 정보:
        - HWPUNIT16: 왼쪽 여백
        - HWPUNIT16: 오른쪽 여백
        - HWPUNIT16: 위쪽 여백
        - HWPUNIT16: 아래쪽 여백
        """
        if len(data) < offset + 8:
            return TableMargin()

        left, right, top, bottom = struct.unpack_from("<HHHH", data, offset)
        return TableMargin(
            left=left * HWPUNIT_TO_PT,
            right=right * HWPUNIT_TO_PT,
            top=top * HWPUNIT_TO_PT,
            bottom=bottom * HWPUNIT_TO_PT,
        )

    def _parse_cell_list_header(self, record: Record) -> Cell | None:
        """LIST_HEADER 레코드에서 셀 정보를 파싱합니다.

        LIST_HEADER는 문단 리스트 헤더(6바이트) + 셀 속성(26바이트)로 구성됩니다.

        셀 속성 (표 80):
        - UINT16: Column 주소
        - UINT16: Row 주소
        - UINT16: 열 병합 개수
        - UINT16: 행 병합 개수
        - HWPUNIT: 셀 폭
        - HWPUNIT: 셀 높이
        - HWPUNIT16[4]: 셀 4방향 여백
        - UINT16: 테두리/배경 ID
        """
        data = record.data

        # 문단 리스트 헤더 건너뛰기 (8바이트)
        # - INT16: 문단 수 (2바이트)
        # - UINT32: 속성 (4바이트)
        # - UINT16: 알 수 없는 필드 (2바이트) - 바이트 덤프로 확인
        list_header_size = 8
        # 셀 속성: 2+2+2+2+4+4+8+2 = 26바이트, 총 34바이트 필요
        # 일부 문서에서는 border_fill_id가 없을 수 있으므로 최소 32바이트만 필요
        if len(data) < list_header_size + 24:
            return None

        offset = list_header_size

        # 셀 속성 파싱 (26바이트) - HWP 스펙 표 80
        # Column 주소 (맨 왼쪽 셀이 0부터 시작)
        col = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # Row 주소 (맨 위쪽 셀이 0부터 시작)
        row = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        colspan = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        rowspan = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # HWPUNIT (4바이트)
        width_raw = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        height_raw = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 셀 4방향 여백 (HWPUNIT16 × 4)
        margins = struct.unpack_from("<HHHH", data, offset)
        offset += 8

        # 테두리/배경 ID (선택적 - 일부 문서에서는 없을 수 있음)
        border_fill_id = 0
        if len(data) >= offset + 2:
            border_fill_id = struct.unpack_from("<H", data, offset)[0]

        return Cell(
            col=col,
            row=row,
            colspan=colspan if colspan > 0 else 1,
            rowspan=rowspan if rowspan > 0 else 1,
            width=width_raw * HWPUNIT_TO_PT,
            height=height_raw * HWPUNIT_TO_PT,
            margin=TableMargin(
                left=margins[0] * HWPUNIT_TO_PT,
                right=margins[1] * HWPUNIT_TO_PT,
                top=margins[2] * HWPUNIT_TO_PT,
                bottom=margins[3] * HWPUNIT_TO_PT,
            ),
            border_fill_id=border_fill_id,
        )

    def _is_table_end(
        self, records: list[Record], current_idx: int, table: Table
    ) -> bool:
        """테이블이 종료되었는지 확인합니다.

        테이블 종료 조건:
        1. 다음 레코드가 형제/부모 레벨 테이블 시작 (level <= 현재 테이블)
        2. level <= 현재 테이블의 start_level인 PARA_HEADER가 나타남
        3. level <= 현재 테이블의 start_level인 LIST_HEADER가 나타남 (중첩 테이블 종료)

        중첩 테이블(level > 현재 테이블)은 종료 조건이 아님.
        """
        # 테이블 속성이 없으면 아직 파싱 중
        if table.row_count == 0 or table.col_count == 0:
            return False

        # 다음 레코드 확인
        if current_idx + 1 < len(records):
            next_record = records[current_idx + 1]

            # 새로운 테이블 시작 확인
            if next_record.tag_id == HwpTagId.CTRL_HEADER:
                ctrl_id = self._parse_ctrl_id(next_record)
                if ctrl_id == CTRL_ID_TABLE:
                    # 다음 테이블의 level이 현재 테이블보다 높으면 중첩 테이블
                    # → 현재 테이블 종료하지 않음
                    if next_record.level > table.start_level:
                        return False
                    # 같거나 낮으면 형제/부모 레벨 테이블 → 현재 테이블 종료
                    return True

            # level <= start_level인 PARA_HEADER면 테이블 종료
            if next_record.tag_id == HwpTagId.PARA_HEADER:
                if next_record.level <= table.start_level:
                    return True

            # level <= start_level인 LIST_HEADER면 테이블 종료 (중첩 테이블에서 외부 테이블로 복귀)
            if next_record.tag_id == HwpTagId.LIST_HEADER:
                if next_record.level <= table.start_level:
                    return True

        return False

    def _parse_para_text(self, record: Record) -> str:
        """PARA_TEXT 레코드에서 텍스트를 추출합니다.

        HWP 제어 문자 처리:
        - 0x00: NULL (종료)
        - 0x01-0x09: 확장 제어 문자 (12바이트 추가 정보)
        - 0x0A (10): 줄바꿈
        - 0x0B (11): GSO 앵커 (표, 그리기 개체 등) - 12바이트 추가
        - 0x0C-0x17: 확장 제어 문자
        - 0x18 (24): 탭 또는 하이픈
        - 0x1E (30): 줄 나눔 없는 빈칸
        """
        data = record.data
        if len(data) < 2:
            return ""

        # 확장 제어 문자 (12바이트 추가 정보가 있는 것들)
        # HWP 5.0 스펙: 1-9, 11-12, 14-23
        EXTENDED_CTRL_CHARS = frozenset([
            1, 2, 3, 4, 5, 6, 7, 8, 9,
            11, 12, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23
        ])

        chars: list[str] = []
        i = 0

        while i + 1 < len(data):
            code = struct.unpack_from("<H", data, i)[0]
            i += 2

            if code >= 0x20:
                chars.append(chr(code))
            elif code == 0:
                break
            elif code == 9:  # TAB: 확장 제어 + 탭 출력
                chars.append("\t")
                i += 14  # 12바이트 추가 정보 + 2바이트 종료 마커
            elif code in EXTENDED_CTRL_CHARS:
                # 확장 제어 문자: 12바이트 추가 정보 + 2바이트 종료 마커
                # 총 16바이트 중 2바이트(제어 코드)는 이미 소비됨
                i += 14
            elif code in (10, 13):
                chars.append("\n")
            elif code == 24:
                chars.append("-")  # 하이픈 (code 24)
            elif code == 30:
                chars.append(" ")  # NBSP

        # 특수 문자 변환 (PUA → 표준 유니코드)
        text = "".join(chars).strip()
        return decode_special_chars(text)

    def _parse_first_char_shape_id(self, record: Record) -> int:
        """PARA_CHAR_SHAPE 레코드에서 첫 번째 글자 모양 ID를 추출합니다.

        PARA_CHAR_SHAPE 구조:
        - (UINT32 pos, UINT32 shape_id) 쌍의 배열
        - 첫 번째 쌍의 shape_id를 반환
        """
        data = record.data
        if len(data) < 8:  # 최소 1개 쌍 (8바이트)
            return 0

        # 첫 번째 쌍: pos(4) + shape_id(4)
        # pos는 0이어야 첫 글자부터 적용
        shape_id = struct.unpack_from("<I", data, 4)[0]
        return shape_id

    def _parse_picture_bin_id(self, record: Record) -> int | None:
        """SHAPE_COMPONENT_PICTURE 레코드에서 bin_id를 추출합니다.

        HWP SHAPE_COMPONENT_PICTURE 레코드 구조:
            - offset 71: BIN 파일 번호 (1-indexed, UINT16)

        bin_id는 BIN 파일 번호에서 1을 뺀 값입니다.
        예: BIN0001.bmp → bin_item_id=1 → bin_id=0

        Args:
            record: SHAPE_COMPONENT_PICTURE 레코드

        Returns:
            bin_id (0-indexed) 또는 파싱 실패 시 None
        """
        data = record.data
        if len(data) < 73:
            return None

        # BIN 파일 번호 (1-indexed)
        bin_item_id = struct.unpack_from("<H", data, 71)[0]

        # 0이면 유효하지 않음
        if bin_item_id == 0:
            return None

        # bin_id는 0-indexed
        return bin_item_id - 1

    def extract_table_text(self, table: Table) -> list[list[str]]:
        """표에서 텍스트를 2D 배열로 추출합니다.

        Args:
            table: Table 객체

        Returns:
            행/열 형태의 텍스트 배열
        """
        result: list[list[str]] = []

        for row_idx in range(table.row_count):
            row_texts: list[str] = []
            for col_idx in range(table.col_count):
                cell = table.get_cell(row_idx, col_idx)
                if cell and cell.row == row_idx and cell.col == col_idx:
                    # 셀의 문단 텍스트 결합
                    cell_text = self._extract_cell_text(cell)
                    row_texts.append(cell_text)
                elif cell:
                    # 병합된 셀의 일부 - 빈 문자열
                    row_texts.append("")
                else:
                    row_texts.append("")
            result.append(row_texts)

        return result

    def _extract_cell_text(self, cell: Cell) -> str:
        """셀에서 텍스트를 추출합니다."""
        texts: list[str] = []
        for para in cell.paragraphs:
            if hasattr(para, "text"):
                texts.append(para.text)
            elif isinstance(para, str):
                texts.append(para)
            elif isinstance(para, Table):
                # 중첩 테이블의 텍스트도 추출
                texts.append(self._extract_nested_table_text(para))
        return "\n".join(texts)

    def _extract_nested_table_text(self, table: Table) -> str:
        """중첩 테이블을 HTML 테이블로 변환합니다."""
        from html import escape

        lines: list[str] = ['<table class="nested-table">']
        lines.append("  <tbody>")

        for row_idx in range(table.row_count):
            lines.append("    <tr>")
            for col_idx in range(table.col_count):
                cell = table.get_cell(row_idx, col_idx)
                if cell and cell.row == row_idx and cell.col == col_idx:
                    # 셀 텍스트 추출 (재귀적으로 중첩 테이블 처리)
                    cell_text = self._extract_cell_text(cell)
                    # 중첩 테이블 HTML이 아닌 경우에만 escape
                    if "<table" not in cell_text:
                        cell_text = escape(cell_text) if cell_text else ""
                        cell_text = cell_text.replace("\n", "<br>")

                    # 병합 속성
                    attrs = ""
                    if cell.rowspan > 1:
                        attrs += f' rowspan="{cell.rowspan}"'
                    if cell.colspan > 1:
                        attrs += f' colspan="{cell.colspan}"'

                    lines.append(f"      <td{attrs}>{cell_text}</td>")
            lines.append("    </tr>")

        lines.append("  </tbody>")
        lines.append("</table>")
        return "\n".join(lines)
