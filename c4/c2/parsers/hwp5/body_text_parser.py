"""BodyTextParser - HWP 5.0 본문 텍스트 파서.

BodyText/Section* 스트림에서 문단과 텍스트를 파싱합니다.

참조:
- HWP 5.0 스펙 4.3절: '본문'의 데이터 레코드
"""

import struct
from dataclasses import dataclass, field
from enum import IntEnum
from typing import ClassVar

from .record_parser import HwpTagId, Record, RecordParser


def make_ctrl_id(a: str, b: str, c: str, d: str) -> int:
    """4문자로 컨트롤 ID를 생성합니다.

    MAKE_4CHID(a, b, c, d) = ((a) << 24) | ((b) << 16) | ((c) << 8) | (d)
    """
    return (ord(a) << 24) | (ord(b) << 16) | (ord(c) << 8) | ord(d)


class CtrlChar(IntEnum):
    """HWP 제어 문자 코드.

    0x00 ~ 0x1F 범위의 문자는 제어 문자로 사용됩니다.
    """

    # 문자 단위 제어 문자 (1 WCHAR = 2 bytes)
    NULL = 0  # 사용 안함
    LINE_BREAK = 10  # 강제 줄 나눔
    PARA_BREAK = 13  # 문단 끝
    TAB = 9  # 탭
    HYPHEN = 24  # 하이픈
    NBSP = 30  # 줄 나눔 없는 빈칸 (NBSP)
    FWSP = 31  # 고정폭 빈칸

    # 인라인 제어 문자 (8 WCHAR = 16 bytes) - 추가 정보 포함
    SECTION_DEF = 2  # 구역 정의
    COLUMN_DEF = 3  # 단 정의
    FIELD_START = 4  # 필드 시작
    FIELD_END = 5  # 필드 끝
    BOOKMARK = 6  # 책갈피
    HEADER_FOOTER = 7  # 머리말/꼬리말
    FOOTNOTE = 8  # 각주/미주
    AUTO_NUM = 9  # 자동 번호

    # 확장 제어 문자 (8 WCHAR = 16 bytes)
    PAGE_CTRL = 11  # 쪽 번호 위치
    PAGE_NUM = 12  # 쪽 번호 제어

    # GSO (Graphic Shape Object) 앵커 (8 WCHAR = 16 bytes)
    GSO_ANCHOR = 11  # 그리기 개체, 표, 수식 등


# 확장/인라인 제어 문자 (16바이트: ctrl_char + 12bytes_info + ctrl_char)
# HWP 스펙 표 6 참조:
# - Extended: 1, 2, 3, 11, 12, 14, 15, 16, 17, 18, 21, 22, 23
# - Inline: 4, 5, 6, 7, 8, 9, 19, 20
# - Char (2바이트만): 0, 10, 13, 24-31
EXTENDED_CTRL_CHARS = frozenset([
    1, 2, 3,           # 예약, 구역/단 정의, 필드 시작
    4, 5, 6, 7, 8, 9,  # 필드 끝, 예약, title mark, 탭
    11, 12, 14,        # 그리기개체/표, 예약
    15, 16, 17, 18,    # 숨은설명, 머리말/꼬리말, 각주/미주, 자동번호
    19, 20, 21,        # 예약, 페이지 컨트롤
    22, 23             # 책갈피, 덧말/글자겹침
])

# 인라인 확장 제어: 총 8 WCHAR (16 bytes)
# [ctrl_char(2)] + [추가정보(12)] + [ctrl_char(2)]
INLINE_EXTENDED_SIZE = 16  # bytes


class CtrlId:
    """HWP 컨트롤 ID 상수.

    MAKE_4CHID(a, b, c, d) 매크로로 생성된 32비트 ID.
    """

    # 표
    TABLE: ClassVar[int] = make_ctrl_id("t", "b", "l", " ")  # 'tbl '

    # 그리기 개체
    LINE: ClassVar[int] = make_ctrl_id("$", "l", "i", "n")  # '$lin'
    RECTANGLE: ClassVar[int] = make_ctrl_id("$", "r", "e", "c")  # '$rec'
    ELLIPSE: ClassVar[int] = make_ctrl_id("$", "e", "l", "l")  # '$ell'
    ARC: ClassVar[int] = make_ctrl_id("$", "a", "r", "c")  # '$arc'
    POLYGON: ClassVar[int] = make_ctrl_id("$", "p", "o", "l")  # '$pol'
    CURVE: ClassVar[int] = make_ctrl_id("$", "c", "u", "r")  # '$cur'

    # 수식
    EQUATION: ClassVar[int] = make_ctrl_id("e", "q", "e", "d")  # 'eqed'

    # 그림
    PICTURE: ClassVar[int] = make_ctrl_id("$", "p", "i", "c")  # '$pic'

    # OLE
    OLE: ClassVar[int] = make_ctrl_id("$", "o", "l", "e")  # '$ole'

    # 묶음 개체
    CONTAINER: ClassVar[int] = make_ctrl_id("$", "c", "o", "n")  # '$con'

    # 구역 정의
    SECTION_DEF: ClassVar[int] = make_ctrl_id("s", "e", "c", "d")  # 'secd'

    # 단 정의
    COLUMN_DEF: ClassVar[int] = make_ctrl_id("c", "o", "l", "d")  # 'cold'

    # 머리말/꼬리말
    HEADER: ClassVar[int] = make_ctrl_id("h", "e", "a", "d")  # 'head'
    FOOTER: ClassVar[int] = make_ctrl_id("f", "o", "o", "t")  # 'foot'

    # 각주/미주
    FOOTNOTE: ClassVar[int] = make_ctrl_id("f", "n", " ", " ")  # 'fn  '
    ENDNOTE: ClassVar[int] = make_ctrl_id("e", "n", " ", " ")  # 'en  '

    # GSO (Graphic Shape Object) 컨테이너 - 텍스트박스 포함 가능
    # 파일에 ' osg'로 저장됨, little-endian 읽기로 0x67736F20
    GSO: ClassVar[int] = make_ctrl_id("g", "s", "o", " ")  # ' osg'

    # 글상자가 포함될 수 있는 그리기 개체 ID 목록
    TEXTBOX_CAPABLE: ClassVar[frozenset[int]] = frozenset(
        [
            RECTANGLE,
            ELLIPSE,
            ARC,
            POLYGON,
            CURVE,
            GSO,  # GSO 컨테이너도 텍스트 포함 가능
        ]
    )


@dataclass
class ListHeader:
    """문단 리스트 헤더.

    글상자, 표 셀 등의 내부 문단 목록 정보.
    """

    para_count: int = 0  # 문단 수
    text_direction: int = 0  # 텍스트 방향 (0=가로, 1=세로)
    line_wrap: int = 0  # 줄바꿈 방식
    vertical_align: int = 0  # 세로 정렬 (0=top, 1=center, 2=bottom)


@dataclass
class TextBox:
    """글상자(텍스트박스) 정보.

    그리기 개체(사각형, 타원 등) 내부의 텍스트 영역.
    """

    # 글상자 여백
    margin_left: int = 0
    margin_right: int = 0
    margin_top: int = 0
    margin_bottom: int = 0

    # 텍스트 최대 폭
    max_width: int = 0

    # 리스트 헤더
    list_header: ListHeader | None = None

    # 내부 문단들
    paragraphs: list["Paragraph"] = field(default_factory=list)

    @property
    def text(self) -> str:
        """글상자 내 전체 텍스트를 반환합니다."""
        return "\n".join(p.text for p in self.paragraphs if p.text.strip())


class OleObjectType(IntEnum):
    """OLE 개체 종류."""

    UNKNOWN = 0
    EMBEDDED = 1
    LINK = 2
    STATIC = 3
    EQUATION = 4


@dataclass
class Equation:
    """수식 개체 정보.

    한글 수식 스크립트를 포함하는 수식 개체입니다.
    HWPTAG_EQEDIT 레코드에서 파싱됩니다.
    """

    # 수식 스크립트
    script: str = ""  # 한글 수식 스크립트 (EQN 형식 호환)

    # 스크립트 속성
    line_mode: bool = False  # True: 줄 단위, False: 글자 단위

    # 스타일
    font_size: int = 0  # 글자 크기 (HWPUNIT)
    color: int = 0  # 글자 색상 (COLORREF)
    baseline: int = 0  # 베이스라인

    # 메타데이터
    version: str = ""  # 수식 버전 정보
    font_name: str = ""  # 수식 폰트 이름

    @property
    def is_empty(self) -> bool:
        """수식이 비어있는지 확인합니다."""
        return not self.script.strip()


@dataclass
class OleObject:
    """OLE 개체 정보.

    차트, 외부 문서 등을 포함하는 OLE 개체입니다.
    HWPTAG_SHAPE_COMPONENT_OLE 레코드에서 파싱됩니다.
    """

    # 개체 속성
    object_type: OleObjectType = OleObjectType.UNKNOWN
    bin_data_id: int = 0  # BinData 스토리지 ID

    # 크기
    extent_x: int = 0  # x 크기
    extent_y: int = 0  # y 크기

    # 테두리
    border_color: int = 0  # 테두리 색
    border_width: int = 0  # 테두리 두께

    # DVASPECT
    draw_aspect: int = 1  # DVASPECT_CONTENT = 1

    @property
    def is_equation(self) -> bool:
        """수식 타입 OLE인지 확인합니다."""
        return self.object_type == OleObjectType.EQUATION

    @property
    def is_embedded(self) -> bool:
        """임베디드 OLE인지 확인합니다."""
        return self.object_type == OleObjectType.EMBEDDED


@dataclass
class CharShapeRef:
    """문단 내 글자 모양 참조."""

    pos: int  # 글자 모양이 바뀌는 시작 위치 (문자 단위)
    shape_id: int  # 글자 모양 ID (DocInfo의 CharShape 인덱스)


@dataclass
class Paragraph:
    """문단 정보."""

    # 문단 헤더 정보
    char_count: int = 0  # 텍스트 문자 수
    para_shape_id: int = 0  # 문단 모양 ID
    style_id: int = 0  # 스타일 ID
    column_type: int = 0  # 단 나누기 종류
    instance_id: int = 0  # 문단 고유 ID

    # 텍스트
    text: str = ""  # 디코딩된 텍스트
    raw_text: bytes = b""  # 원본 바이트

    # 글자 모양 참조
    char_shapes: list[CharShapeRef] = field(default_factory=list)

    # 컨트롤/개체 마스크
    control_mask: int = 0

    # 글상자 (있는 경우)
    textboxes: list[TextBox] = field(default_factory=list)

    # 수식 (있는 경우)
    equations: list[Equation] = field(default_factory=list)

    # OLE 개체 (있는 경우)
    ole_objects: list[OleObject] = field(default_factory=list)

    # 레코드 순서 (문서 내 위치)
    record_index: int = 0

    # Y 좌표 (GSO 문단용, 페이지 내 수직 위치)
    y_offset: int = 0

    @property
    def has_controls(self) -> bool:
        """제어 문자가 포함되어 있는지 확인합니다."""
        return self.control_mask != 0

    @property
    def has_textbox(self) -> bool:
        """글상자가 포함되어 있는지 확인합니다."""
        return len(self.textboxes) > 0

    @property
    def has_equation(self) -> bool:
        """수식이 포함되어 있는지 확인합니다."""
        return len(self.equations) > 0

    @property
    def has_ole_object(self) -> bool:
        """OLE 개체가 포함되어 있는지 확인합니다."""
        return len(self.ole_objects) > 0


class BodyTextParser:
    """BodyText 스트림 파서.

    사용법:
        parser = BodyTextParser()
        paragraphs = parser.parse(section_data)

        # 이미지 위치 정보 조회
        gso_positions = parser.gso_positions  # {bin_id: record_index}
    """

    def __init__(self) -> None:
        """BodyTextParser를 초기화합니다."""
        self._record_parser = RecordParser()
        # GSO/PICTURE 앵커 위치 매핑 (bin_id → record_index)
        self.gso_positions: dict[int, int] = {}

    def parse(self, data: bytes) -> list[Paragraph]:
        """BodyText 섹션 데이터를 파싱합니다.

        Args:
            data: BodyText/Section* 스트림 데이터

        Returns:
            Paragraph 리스트
        """
        # 파싱 전 gso_positions 초기화
        self.gso_positions = {}
        records = self._record_parser.parse_records(data, decompress=True)
        return self._parse_records(records)

    def _parse_records(self, records: list[Record]) -> list[Paragraph]:
        """레코드 리스트에서 문단을 추출합니다."""
        paragraphs: list[Paragraph] = []
        current_para: Paragraph | None = None

        # 글상자 파싱 상태
        current_ctrl_id: int | None = None
        current_textbox: TextBox | None = None
        textbox_para_count: int = 0
        textbox_para_collected: int = 0
        in_textbox: bool = False

        # 테이블 파싱 상태 - 테이블 내부 문단은 건너뜀
        in_table: bool = False
        table_cell_para_count: int = 0
        table_cell_para_collected: int = 0

        # GSO (그리기 개체) 텍스트박스 - LIST_HEADER 없이 직접 문단 포함
        in_gso: bool = False
        gso_textbox: TextBox | None = None
        gso_was_in_table: bool = False  # GSO 진입 시 테이블 상태 저장
        gso_start_index: int = 0  # GSO 시작 레코드 인덱스
        gso_y_offset: int = 0  # GSO Y 좌표 (정렬용)
        gso_start_level: int = 0  # GSO 시작 레코드의 level (종료 감지용)

        i = 0
        while i < len(records):
            record = records[i]

            if record.tag_id == HwpTagId.PARA_HEADER:
                # 새 문단 시작
                if in_gso:
                    # GSO 내부 문단 - 텍스트박스로 수집
                    para = self._parse_para_header(record)
                    para.record_index = gso_start_index  # GSO 시작 위치를 문단 순서로 설정
                    para.y_offset = gso_y_offset  # Y 좌표 설정 (정렬용)

                    # GSO 텍스트박스가 없으면 생성
                    if gso_textbox is None:
                        gso_textbox = TextBox(list_header=ListHeader(para_count=1))

                    gso_textbox.paragraphs.append(para)

                    # GSO 내부 문단의 텍스트 처리
                    if i + 1 < len(records) and records[i + 1].tag_id == HwpTagId.PARA_TEXT:
                        i += 1
                        text, raw = self._parse_para_text(records[i])
                        para.text = text
                        para.raw_text = raw

                    # 글자 모양 처리
                    if (
                        i + 1 < len(records)
                        and records[i + 1].tag_id == HwpTagId.PARA_CHAR_SHAPE
                    ):
                        i += 1
                        para.char_shapes = self._parse_char_shapes(records[i])

                elif in_table:
                    # 테이블 내부 문단 처리
                    if record.level == 0:
                        # level=0 문단이면 테이블 종료 - 일반 문단으로 처리
                        in_table = False
                        table_cell_para_count = 0
                        table_cell_para_collected = 0
                        if current_para is not None:
                            paragraphs.append(current_para)
                        current_para = self._parse_para_header(record)
                        current_para.record_index = i
                    else:
                        # 테이블 내부 문단 - 건너뜀 (TableParser에서 처리)
                        table_cell_para_collected += 1
                        if table_cell_para_collected >= table_cell_para_count:
                            # 현재 셀의 문단 처리 완료, 다음 셀 대기
                            table_cell_para_count = 0
                            table_cell_para_collected = 0
                elif in_textbox and current_textbox is not None:
                    # 글상자 내부 문단
                    para = self._parse_para_header(record)
                    current_textbox.paragraphs.append(para)
                    textbox_para_collected += 1

                    # 글상자 내부 문단의 텍스트 처리
                    if i + 1 < len(records) and records[i + 1].tag_id == HwpTagId.PARA_TEXT:
                        i += 1
                        text, raw = self._parse_para_text(records[i])
                        para.text = text
                        para.raw_text = raw

                    # 글자 모양 처리
                    if (
                        i + 1 < len(records)
                        and records[i + 1].tag_id == HwpTagId.PARA_CHAR_SHAPE
                    ):
                        i += 1
                        para.char_shapes = self._parse_char_shapes(records[i])

                    # 글상자 내 모든 문단 수집 완료
                    if textbox_para_collected >= textbox_para_count:
                        in_textbox = False
                        if current_para is not None:
                            current_para.textboxes.append(current_textbox)
                        current_textbox = None
                else:
                    # 일반 문단 - level 0만 최상위 문단으로 처리
                    # level > 0은 컨테이너(테이블, 텍스트박스 등) 내부이므로 건너뜀
                    if record.level == 0:
                        if current_para is not None:
                            paragraphs.append(current_para)
                        current_para = self._parse_para_header(record)
                        current_para.record_index = i

            elif record.tag_id == HwpTagId.PARA_TEXT and current_para is not None:
                if not in_textbox and not in_table:
                    # 일반 문단 텍스트 (테이블/글상자 외부)
                    text, raw = self._parse_para_text(record)
                    current_para.text = text
                    current_para.raw_text = raw

            elif record.tag_id == HwpTagId.PARA_CHAR_SHAPE and current_para is not None:
                if not in_textbox and not in_table:
                    # 일반 문단 글자 모양 참조
                    current_para.char_shapes = self._parse_char_shapes(record)

            elif record.tag_id == HwpTagId.CTRL_HEADER:
                # 컨트롤 헤더 - 컨트롤 ID 추출
                ctrl_id = self._parse_ctrl_id(record)

                # 이전 GSO 텍스트박스 완료 처리
                # 새로운 CTRL_HEADER가 나오면 GSO 상태를 항상 리셋해야 함
                if in_gso:
                    if gso_textbox is not None and gso_textbox.paragraphs:
                        # GSO 텍스트박스의 텍스트를 문단으로 추가
                        for gso_para in gso_textbox.paragraphs:
                            if gso_para.text:
                                paragraphs.append(gso_para)
                    # GSO 상태 항상 리셋 (텍스트박스가 비어있어도)
                    gso_textbox = None
                    in_gso = False
                    # GSO 종료 후 테이블 상태 복원
                    in_table = gso_was_in_table
                    gso_was_in_table = False

                current_ctrl_id = ctrl_id

                # 테이블 시작 감지
                if ctrl_id == CtrlId.TABLE:
                    in_table = True
                    table_cell_para_count = 0
                    table_cell_para_collected = 0
                elif ctrl_id == CtrlId.GSO:
                    # GSO (그리기 개체) - LIST_HEADER 없이 텍스트 포함 가능
                    gso_was_in_table = in_table  # 테이블 상태 저장
                    in_gso = True
                    in_table = False  # GSO 처리 중에는 테이블 문단 건너뛰기 비활성화
                    gso_textbox = None
                    gso_start_index = i  # GSO 시작 레코드 인덱스 저장
                    gso_start_level = record.level  # GSO 시작 level 저장 (종료 감지용)
                    # GSO CTRL_HEADER에서 y_offset 파싱 (offset 8에 위치)
                    gso_y_offset = 0
                    if len(record.data) >= 12:
                        gso_y_offset = struct.unpack_from("<i", record.data, 8)[0]
                elif in_table and ctrl_id != CtrlId.TABLE:
                    # 다른 컨트롤이 나타나면 테이블 종료
                    in_table = False

            elif record.tag_id == HwpTagId.LIST_HEADER:
                # 문단 리스트 헤더
                # GSO 종료 감지: LIST_HEADER의 level이 GSO 시작 level 이하면 GSO 범위 종료
                if in_gso and record.level <= gso_start_level:
                    if gso_textbox is not None and gso_textbox.paragraphs:
                        for gso_para in gso_textbox.paragraphs:
                            if gso_para.text:
                                paragraphs.append(gso_para)
                    gso_textbox = None
                    in_gso = False
                    # GSO 종료 후 테이블 상태 복원
                    in_table = gso_was_in_table
                    gso_was_in_table = False

                if in_table:
                    # 테이블 셀의 문단 리스트 헤더
                    list_header = self._parse_list_header(record)
                    table_cell_para_count = list_header.para_count
                    table_cell_para_collected = 0
                elif current_ctrl_id in CtrlId.TEXTBOX_CAPABLE:
                    # 글상자 내부 문단 시작
                    list_header = self._parse_list_header(record)
                    if list_header.para_count > 0:
                        current_textbox = TextBox(list_header=list_header)
                        textbox_para_count = list_header.para_count
                        textbox_para_collected = 0
                        in_textbox = True

            elif record.tag_id == HwpTagId.SHAPE_COMPONENT:
                # 개체 요소 - 글상자 텍스트 속성이 포함될 수 있음
                # 개체 요소 내에서 컨트롤 ID 갱신
                if len(record.data) >= 4:
                    ctrl_id = struct.unpack_from("<I", record.data, 0)[0]
                    if ctrl_id in CtrlId.TEXTBOX_CAPABLE:
                        current_ctrl_id = ctrl_id

            elif record.tag_id == HwpTagId.EQEDIT:
                # 수식 개체
                equation = self._parse_eqedit(record)
                if current_para is not None and not equation.is_empty:
                    current_para.equations.append(equation)

            elif record.tag_id == HwpTagId.SHAPE_COMPONENT_OLE:
                # OLE 개체
                ole_obj = self._parse_ole_object(record)
                if current_para is not None:
                    current_para.ole_objects.append(ole_obj)

            elif record.tag_id == HwpTagId.SHAPE_COMPONENT_PICTURE:
                # 그림 개체 - bin_id와 record_index 매핑 저장
                bin_id = self._parse_picture_bin_id(record)
                if bin_id is not None:
                    # GSO 시작 인덱스가 있으면 사용, 없으면 현재 인덱스
                    position_index = gso_start_index if in_gso else i
                    self.gso_positions[bin_id] = position_index

            i += 1

        # 마지막 GSO 텍스트박스 처리
        if in_gso and gso_textbox is not None and gso_textbox.paragraphs:
            for gso_para in gso_textbox.paragraphs:
                if gso_para.text:
                    paragraphs.append(gso_para)

        # 마지막 문단 추가
        if current_para is not None:
            paragraphs.append(current_para)

        return paragraphs

    def _parse_para_header(self, record: Record) -> Paragraph:
        """PARA_HEADER 레코드를 파싱합니다."""
        data = record.data
        if len(data) < 16:
            return Paragraph()

        offset = 0

        # 텍스트 문자 수 (UINT32)
        nchars = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 상위 비트가 설정되어 있으면 마스킹
        if nchars & 0x80000000:
            nchars &= 0x7FFFFFFF

        # 컨트롤 마스크 (UINT32)
        control_mask = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 문단 모양 ID (UINT16)
        para_shape_id = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 스타일 ID (UINT8)
        style_id = struct.unpack_from("<B", data, offset)[0]
        offset += 1

        # 단 나누기 종류 (UINT8)
        column_type = struct.unpack_from("<B", data, offset)[0]
        offset += 1

        # 글자 모양 정보 수 (UINT16) - 사용하지 않음
        offset += 2

        # range tag 정보 수 (UINT16) - 사용하지 않음
        offset += 2

        # align 정보 수 (UINT16) - 사용하지 않음
        offset += 2

        # Instance ID (UINT32)
        instance_id = 0
        if len(data) >= offset + 4:
            instance_id = struct.unpack_from("<I", data, offset)[0]

        return Paragraph(
            char_count=nchars,
            para_shape_id=para_shape_id,
            style_id=style_id,
            column_type=column_type,
            control_mask=control_mask,
            instance_id=instance_id,
        )

    def _parse_para_text(self, record: Record) -> tuple[str, bytes]:
        """PARA_TEXT 레코드를 파싱합니다.

        Returns:
            (디코딩된 텍스트, 원본 바이트) 튜플
        """
        data = record.data
        if not data:
            return "", b""

        # WCHAR 배열로 처리 (2바이트씩)
        text_parts: list[str] = []
        offset = 0

        while offset < len(data):
            if offset + 2 > len(data):
                break

            # 2바이트 읽기 (WCHAR)
            char_code = struct.unpack_from("<H", data, offset)[0]

            # 제어 문자 처리 (0x00 ~ 0x1F)
            if char_code < 0x20:
                if char_code in EXTENDED_CTRL_CHARS:
                    # 확장/인라인 제어 문자: 16바이트 건너뛰기
                    # [ctrl_code(2)] + [param(12)] + [end_marker(2)]
                    # 특수 처리: TAB은 탭 문자 출력
                    if char_code == CtrlChar.TAB:
                        text_parts.append("\t")
                    offset += INLINE_EXTENDED_SIZE
                    continue
                elif char_code == CtrlChar.LINE_BREAK:
                    text_parts.append("\n")
                elif char_code == CtrlChar.PARA_BREAK:
                    # 문단 끝 - 무시
                    pass
                elif char_code == CtrlChar.NBSP:
                    text_parts.append("\u00A0")  # Non-breaking space
                elif char_code == CtrlChar.FWSP:
                    text_parts.append(" ")  # 고정폭 빈칸을 일반 공백으로
                # 그 외 제어 문자는 무시 (0, 24-31)
                offset += 2
            else:
                # 일반 문자
                try:
                    char = chr(char_code)
                    text_parts.append(char)
                except (ValueError, OverflowError):
                    # 잘못된 유니코드는 대체 문자로
                    text_parts.append("\ufffd")
                offset += 2

        # 특수 문자 변환 (PUA → 표준 유니코드)
        text = self.decode_special_chars("".join(text_parts))
        return text, data

    def _parse_char_shapes(self, record: Record) -> list[CharShapeRef]:
        """PARA_CHAR_SHAPE 레코드를 파싱합니다."""
        data = record.data
        char_shapes: list[CharShapeRef] = []

        # 8바이트씩 읽기 (pos: UINT32, shape_id: UINT32)
        offset = 0
        while offset + 8 <= len(data):
            pos = struct.unpack_from("<I", data, offset)[0]
            shape_id = struct.unpack_from("<I", data, offset + 4)[0]
            char_shapes.append(CharShapeRef(pos=pos, shape_id=shape_id))
            offset += 8

        return char_shapes

    def _parse_ctrl_id(self, record: Record) -> int:
        """CTRL_HEADER 레코드에서 컨트롤 ID를 추출합니다.

        Args:
            record: CTRL_HEADER 레코드

        Returns:
            컨트롤 ID (32비트 정수)
        """
        data = record.data
        if len(data) < 4:
            return 0

        return struct.unpack_from("<I", data, 0)[0]

    def _parse_list_header(self, record: Record) -> ListHeader:
        """LIST_HEADER 레코드를 파싱합니다.

        문단 리스트 헤더는 글상자, 표 셀 등의 내부 문단 정보를 담고 있습니다.

        Args:
            record: LIST_HEADER 레코드

        Returns:
            ListHeader 객체
        """
        data = record.data
        if len(data) < 6:
            return ListHeader()

        offset = 0

        # 문단 수 (INT16)
        para_count = struct.unpack_from("<h", data, offset)[0]
        offset += 2

        # 속성 (UINT32)
        properties = struct.unpack_from("<I", data, offset)[0]

        # 속성 비트 추출
        text_direction = properties & 0x07  # bit 0~2
        line_wrap = (properties >> 3) & 0x03  # bit 3~4
        vertical_align = (properties >> 5) & 0x03  # bit 5~6

        return ListHeader(
            para_count=para_count,
            text_direction=text_direction,
            line_wrap=line_wrap,
            vertical_align=vertical_align,
        )

    def decode_special_chars(self, text: str) -> str:
        """특수 문자를 처리합니다.

        원문자, 로마숫자 등의 특수 문자를 유니코드로 변환합니다.

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

    def extract_plain_text(
        self, paragraphs: list[Paragraph], include_textboxes: bool = True
    ) -> str:
        """문단 리스트에서 순수 텍스트를 추출합니다.

        Args:
            paragraphs: Paragraph 리스트
            include_textboxes: 글상자 텍스트 포함 여부

        Returns:
            줄바꿈으로 연결된 텍스트
        """
        texts = []
        for para in paragraphs:
            text = para.text.strip()
            if text:
                texts.append(text)

            # 글상자 텍스트 추출
            if include_textboxes:
                for textbox in para.textboxes:
                    textbox_text = textbox.text.strip()
                    if textbox_text:
                        texts.append(textbox_text)

        return "\n".join(texts)

    def extract_textboxes(self, paragraphs: list[Paragraph]) -> list[TextBox]:
        """문단 리스트에서 모든 글상자를 추출합니다.

        Args:
            paragraphs: Paragraph 리스트

        Returns:
            TextBox 리스트
        """
        textboxes = []
        for para in paragraphs:
            textboxes.extend(para.textboxes)
        return textboxes

    def _parse_eqedit(self, record: Record) -> Equation:
        """EQEDIT 레코드를 파싱합니다.

        한글 수식 개체는 EQN 형식의 수식 스크립트를 포함합니다.

        레코드 구조:
            - UINT32: 속성 (bit 0 = 줄 단위 모드)
            - WORD: 스크립트 길이 (바이트 단위)
            - WCHAR[n]: 수식 스크립트 (UTF-16LE)
            - HWPUNIT (INT32): 글자 크기
            - COLORREF (UINT32): 글자 색상
            - INT16: 베이스라인
            - (옵션) WORD + WCHAR[]: 버전 문자열
            - (옵션) WORD + WCHAR[]: 폰트 이름

        Args:
            record: EQEDIT 레코드

        Returns:
            Equation 객체
        """
        data = record.data
        if len(data) < 6:  # 최소 속성(4) + 길이(2)
            return Equation()

        offset = 0

        # 속성 (UINT32)
        properties = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        line_mode = bool(properties & 0x01)

        # 스크립트 길이 (WORD = UINT16, 바이트 단위)
        script_length = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 수식 스크립트 (WCHAR 배열 = UTF-16LE)
        script = ""
        if script_length > 0 and offset + script_length <= len(data):
            try:
                script_bytes = data[offset : offset + script_length]
                script = script_bytes.decode("utf-16le", errors="replace")
            except (UnicodeDecodeError, ValueError):
                script = ""
            offset += script_length

        # 글자 크기 (HWPUNIT = INT32)
        font_size = 0
        if offset + 4 <= len(data):
            font_size = struct.unpack_from("<i", data, offset)[0]
            offset += 4

        # 글자 색상 (COLORREF = UINT32)
        color = 0
        if offset + 4 <= len(data):
            color = struct.unpack_from("<I", data, offset)[0]
            offset += 4

        # 베이스라인 (INT16)
        baseline = 0
        if offset + 2 <= len(data):
            baseline = struct.unpack_from("<h", data, offset)[0]
            offset += 2

        # 버전 문자열 (옵션: WORD 길이 + WCHAR[])
        version = ""
        if offset + 2 <= len(data):
            version_length = struct.unpack_from("<H", data, offset)[0]
            offset += 2
            if version_length > 0 and offset + version_length * 2 <= len(data):
                try:
                    version_bytes = data[offset : offset + version_length * 2]
                    version = version_bytes.decode("utf-16le", errors="replace")
                except (UnicodeDecodeError, ValueError):
                    version = ""
                offset += version_length * 2

        # 폰트 이름 (옵션: WORD 길이 + WCHAR[])
        font_name = ""
        if offset + 2 <= len(data):
            font_name_length = struct.unpack_from("<H", data, offset)[0]
            offset += 2
            if font_name_length > 0 and offset + font_name_length * 2 <= len(data):
                try:
                    font_name_bytes = data[offset : offset + font_name_length * 2]
                    font_name = font_name_bytes.decode("utf-16le", errors="replace")
                except (UnicodeDecodeError, ValueError):
                    font_name = ""

        return Equation(
            script=script,
            line_mode=line_mode,
            font_size=font_size,
            color=color,
            baseline=baseline,
            version=version,
            font_name=font_name,
        )

    def _parse_ole_object(self, record: Record) -> OleObject:
        """SHAPE_COMPONENT_OLE 레코드를 파싱합니다.

        OLE 개체는 외부 문서, 차트, 수식 등을 포함합니다.

        레코드 구조:
            - UINT32: 속성 (bits 0-2: object type)
            - INT32: extent_x (x 크기)
            - INT32: extent_y (y 크기)
            - UINT16: BinData ID
            - COLORREF (UINT32): 테두리 색
            - INT32: 테두리 두께
            - UINT32: DVASPECT

        Args:
            record: SHAPE_COMPONENT_OLE 레코드

        Returns:
            OleObject 객체
        """
        data = record.data
        if len(data) < 4:
            return OleObject()

        offset = 0

        # 속성 (UINT32)
        # HWP 스펙: bit 0-2에 개체 종류가 저장됨
        properties = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 개체 타입 추출 (하위 3비트)
        object_type_value = properties & 0x07
        try:
            object_type = OleObjectType(object_type_value)
        except ValueError:
            object_type = OleObjectType.UNKNOWN

        # extent_x (INT32)
        extent_x = 0
        if offset + 4 <= len(data):
            extent_x = struct.unpack_from("<i", data, offset)[0]
            offset += 4

        # extent_y (INT32)
        extent_y = 0
        if offset + 4 <= len(data):
            extent_y = struct.unpack_from("<i", data, offset)[0]
            offset += 4

        # BinData ID (UINT16)
        bin_data_id = 0
        if offset + 2 <= len(data):
            bin_data_id = struct.unpack_from("<H", data, offset)[0]
            offset += 2

        # 테두리 색 (COLORREF = UINT32)
        border_color = 0
        if offset + 4 <= len(data):
            border_color = struct.unpack_from("<I", data, offset)[0]
            offset += 4

        # 테두리 두께 (INT32)
        border_width = 0
        if offset + 4 <= len(data):
            border_width = struct.unpack_from("<i", data, offset)[0]
            offset += 4

        # DVASPECT (UINT32)
        draw_aspect = 1  # 기본값: DVASPECT_CONTENT
        if offset + 4 <= len(data):
            draw_aspect = struct.unpack_from("<I", data, offset)[0]

        return OleObject(
            object_type=object_type,
            bin_data_id=bin_data_id,
            extent_x=extent_x,
            extent_y=extent_y,
            border_color=border_color,
            border_width=border_width,
            draw_aspect=draw_aspect,
        )

    def extract_equations(self, paragraphs: list[Paragraph]) -> list[Equation]:
        """문단 리스트에서 모든 수식을 추출합니다.

        Args:
            paragraphs: Paragraph 리스트

        Returns:
            Equation 리스트
        """
        equations = []
        for para in paragraphs:
            equations.extend(para.equations)
        return equations

    def extract_ole_objects(self, paragraphs: list[Paragraph]) -> list[OleObject]:
        """문단 리스트에서 모든 OLE 개체를 추출합니다.

        Args:
            paragraphs: Paragraph 리스트

        Returns:
            OleObject 리스트
        """
        ole_objects = []
        for para in paragraphs:
            ole_objects.extend(para.ole_objects)
        return ole_objects

    def _parse_picture_bin_id(self, record: Record) -> int | None:
        """SHAPE_COMPONENT_PICTURE 레코드에서 bin_id를 추출합니다.

        HWP SHAPE_COMPONENT_PICTURE 레코드 구조 (실제 분석 결과):
            - offset 0-70: 크기, 위치 등 그래픽 속성 (대부분 0)
            - offset 71: BIN 파일 번호 (1-indexed, UINT16)
            - offset 73-89: 기타 속성

        bin_id는 BIN 파일 번호에서 1을 뺀 값입니다.
        예: BIN0001.bmp → bin_item_id=1 → bin_id=0

        Args:
            record: SHAPE_COMPONENT_PICTURE 레코드

        Returns:
            bin_id (0-indexed) 또는 파싱 실패 시 None
        """
        data = record.data
        # BIN 파일 번호는 offset 71에 위치
        if len(data) < 73:
            return None

        # BIN 파일 번호 (1-indexed)
        bin_item_id = struct.unpack_from("<H", data, 71)[0]

        # 0이면 유효하지 않음
        if bin_item_id == 0:
            return None

        # bin_id는 0-indexed
        return bin_item_id - 1
