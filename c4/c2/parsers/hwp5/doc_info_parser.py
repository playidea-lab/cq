"""DocInfoParser - HWP 5.0 문서 정보 파서.

DocInfo 스트림에서 글자 모양, 문단 모양, 글꼴 정보를 파싱합니다.

참조:
- HWP 5.0 스펙 4.2절: '문서 정보'의 데이터 레코드
"""

import struct
from dataclasses import dataclass, field
from enum import IntEnum

from .record_parser import HwpTagId, Record, RecordParser

# HWP 단위: 1 HWPUNIT = 1/7200 inch
# 1 inch = 72 pt, 따라서 1 HWPUNIT = 72/7200 = 0.01 pt
HWPUNIT_TO_PT = 72 / 7200  # 0.01


class TextAlignment(IntEnum):
    """문단 정렬 방식."""

    JUSTIFY = 0  # 양쪽 정렬
    LEFT = 1  # 왼쪽 정렬
    RIGHT = 2  # 오른쪽 정렬
    CENTER = 3  # 가운데 정렬
    DISTRIBUTE = 4  # 배분 정렬
    DIVIDE = 5  # 나눔 정렬


class LineSpacingType(IntEnum):
    """줄 간격 종류."""

    PERCENT = 0  # 글자에 따라 (%)
    FIXED = 1  # 고정값
    SPACE_ONLY = 2  # 여백만 지정
    MINIMUM = 3  # 최소


@dataclass
class CharShape:
    """글자 모양 정보."""

    # 언어별 글꼴 ID (한글, 영어, 한자, 일어, 기타, 기호, 사용자)
    face_ids: list[int] = field(default_factory=lambda: [0] * 7)

    # 기준 크기 (pt 단위)
    font_size: float = 10.0

    # 속성
    italic: bool = False
    bold: bool = False
    underline_type: int = 0  # 0=없음, 1=글자 아래, 3=글자 위
    underline_shape: int = 0
    outline_type: int = 0  # 0=없음, 1=실선, 2=점선, ...
    shadow_type: int = 0  # 0=없음, 1=비연속, 2=연속
    emboss: bool = False  # 양각
    engrave: bool = False  # 음각
    superscript: bool = False  # 위 첨자
    subscript: bool = False  # 아래 첨자
    strikeout: int = 0  # 취소선
    emphasis_type: int = 0  # 강조점 종류
    kerning: bool = False

    # 색상 (ARGB 형식)
    text_color: int = 0x00000000  # 글자 색
    underline_color: int = 0x00000000  # 밑줄 색
    shade_color: int = 0xFFFFFFFF  # 음영 색
    shadow_color: int = 0x00B2B2B2  # 그림자 색

    # 장평/자간
    char_widths: list[int] = field(default_factory=lambda: [100] * 7)  # 장평 %
    char_spacings: list[int] = field(default_factory=lambda: [0] * 7)  # 자간 %

    @property
    def font_size_pt(self) -> float:
        """폰트 크기를 pt 단위로 반환합니다."""
        return self.font_size

    @classmethod
    def from_record(cls, record: Record) -> "CharShape":
        """Record에서 CharShape를 파싱합니다."""
        data = record.data
        if len(data) < 72:
            raise ValueError(f"CharShape 데이터가 너무 짧습니다: {len(data)} bytes")

        offset = 0

        # 언어별 글꼴 ID (WORD * 7 = 14 bytes)
        face_ids = list(struct.unpack_from("<7H", data, offset))
        offset += 14

        # 언어별 장평 (UINT8 * 7 = 7 bytes)
        char_widths = list(struct.unpack_from("<7B", data, offset))
        offset += 7

        # 언어별 자간 (INT8 * 7 = 7 bytes)
        char_spacings = list(struct.unpack_from("<7b", data, offset))
        offset += 7

        # 언어별 상대 크기 (UINT8 * 7 = 7 bytes) - 스킵
        offset += 7

        # 언어별 글자 위치 (INT8 * 7 = 7 bytes) - 스킵
        offset += 7

        # 기준 크기 (INT32, HWPUNIT)
        base_size_hwpunit = struct.unpack_from("<i", data, offset)[0]
        offset += 4
        # HWPUNIT을 pt로 변환
        font_size = base_size_hwpunit * HWPUNIT_TO_PT

        # 속성 (UINT32)
        attrs = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 속성 비트 파싱
        italic = bool(attrs & 0x01)
        bold = bool(attrs & 0x02)
        underline_type = (attrs >> 2) & 0x03
        underline_shape = (attrs >> 4) & 0x0F
        outline_type = (attrs >> 8) & 0x07
        shadow_type = (attrs >> 11) & 0x03
        emboss = bool(attrs & (1 << 13))
        engrave = bool(attrs & (1 << 14))
        superscript = bool(attrs & (1 << 15))
        subscript = bool(attrs & (1 << 16))
        strikeout = (attrs >> 18) & 0x07
        emphasis_type = (attrs >> 21) & 0x0F
        kerning = bool(attrs & (1 << 30))

        # 그림자 간격 (INT8 * 2 = 2 bytes) - 스킵
        offset += 2

        # 글자 색 (COLORREF = 4 bytes)
        text_color = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 밑줄 색
        underline_color = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 음영 색
        shade_color = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 그림자 색
        shadow_color = struct.unpack_from("<I", data, offset)[0]

        return cls(
            face_ids=face_ids,
            font_size=font_size,
            italic=italic,
            bold=bold,
            underline_type=underline_type,
            underline_shape=underline_shape,
            outline_type=outline_type,
            shadow_type=shadow_type,
            emboss=emboss,
            engrave=engrave,
            superscript=superscript,
            subscript=subscript,
            strikeout=strikeout,
            emphasis_type=emphasis_type,
            kerning=kerning,
            text_color=text_color,
            underline_color=underline_color,
            shade_color=shade_color,
            shadow_color=shadow_color,
            char_widths=char_widths,
            char_spacings=char_spacings,
        )


@dataclass
class ParaShape:
    """문단 모양 정보."""

    # 여백 (HWPUNIT → pt 변환 필요)
    left_margin: float = 0.0  # 왼쪽 여백 (pt)
    right_margin: float = 0.0  # 오른쪽 여백 (pt)
    indent: float = 0.0  # 들여쓰기/내어쓰기 (pt)
    spacing_before: float = 0.0  # 문단 간격 위 (pt)
    spacing_after: float = 0.0  # 문단 간격 아래 (pt)

    # 줄 간격
    line_spacing: float = 160.0  # 줄 간격 값
    line_spacing_type: LineSpacingType = LineSpacingType.PERCENT

    # 정렬
    alignment: TextAlignment = TextAlignment.JUSTIFY

    # 문단 머리 모양
    head_type: int = 0  # 0=없음, 1=개요, 2=번호, 3=글머리표
    para_level: int = 0  # 문단 수준 (1~7)

    # 참조 ID
    tab_def_id: int = 0
    numbering_id: int = 0
    border_fill_id: int = 0

    @classmethod
    def from_record(cls, record: Record) -> "ParaShape":
        """Record에서 ParaShape를 파싱합니다."""
        data = record.data
        if len(data) < 40:  # 최소 크기 체크
            raise ValueError(f"ParaShape 데이터가 너무 짧습니다: {len(data)} bytes")

        offset = 0

        # 속성 1 (UINT32)
        attrs1 = struct.unpack_from("<I", data, offset)[0]
        offset += 4

        # 왼쪽 여백 (INT32, HWPUNIT)
        left_margin = struct.unpack_from("<i", data, offset)[0] * HWPUNIT_TO_PT
        offset += 4

        # 오른쪽 여백
        right_margin = struct.unpack_from("<i", data, offset)[0] * HWPUNIT_TO_PT
        offset += 4

        # 들여쓰기/내어쓰기
        indent = struct.unpack_from("<i", data, offset)[0] * HWPUNIT_TO_PT
        offset += 4

        # 문단 간격 위
        spacing_before = struct.unpack_from("<i", data, offset)[0] * HWPUNIT_TO_PT
        offset += 4

        # 문단 간격 아래
        spacing_after = struct.unpack_from("<i", data, offset)[0] * HWPUNIT_TO_PT
        offset += 4

        # 줄 간격 (구버전)
        line_spacing_old = struct.unpack_from("<i", data, offset)[0]
        offset += 4

        # 탭 정의 ID
        tab_def_id = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 번호 문단 ID
        numbering_id = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 테두리/배경 ID
        border_fill_id = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 속성 1 파싱
        line_spacing_type_old = attrs1 & 0x03  # bit 0~1
        alignment = TextAlignment((attrs1 >> 2) & 0x07)  # bit 2~4
        head_type = (attrs1 >> 23) & 0x03  # bit 23~24
        para_level = (attrs1 >> 25) & 0x07  # bit 25~27

        # 줄 간격 처리
        # 새 버전 (5.0.2.5 이상)에서는 속성 3과 별도 줄 간격 필드가 있음
        line_spacing_type = LineSpacingType(line_spacing_type_old)
        line_spacing = float(line_spacing_old)

        # 데이터가 더 있으면 새 버전 필드 읽기
        if len(data) >= 54:
            # 문단 테두리 간격 (INT16 * 4 = 8 bytes)
            offset += 8

            # 속성 2 (UINT32)
            offset += 4

            # 속성 3 (UINT32)
            attrs3 = struct.unpack_from("<I", data, offset)[0]
            offset += 4

            # 줄 간격 (UINT32, 새 버전)
            new_line_spacing = struct.unpack_from("<I", data, offset)[0]

            # 새 버전 줄 간격 종류 (속성 3의 bit 0~4)
            new_line_spacing_type = attrs3 & 0x1F
            if new_line_spacing_type <= 3:
                line_spacing_type = LineSpacingType(new_line_spacing_type)
                line_spacing = float(new_line_spacing)

        return cls(
            left_margin=left_margin,
            right_margin=right_margin,
            indent=indent,
            spacing_before=spacing_before,
            spacing_after=spacing_after,
            line_spacing=line_spacing,
            line_spacing_type=line_spacing_type,
            alignment=alignment,
            head_type=head_type,
            para_level=para_level,
            tab_def_id=tab_def_id,
            numbering_id=numbering_id,
            border_fill_id=border_fill_id,
        )


@dataclass
class BorderFill:
    """테두리/배경 정보.

    HWP 5.0 스펙 표 21 참조.
    """

    # 배경색 (COLORREF 형식, ARGB)
    background_color: int | None = None  # None이면 배경 없음

    # 속성
    three_d: bool = False  # 3D 효과
    shadow: bool = False  # 그림자

    # 테두리 유형 (0=없음, 1=실선, 2=파선, ...)
    left_border_type: int = 0
    right_border_type: int = 0
    top_border_type: int = 0
    bottom_border_type: int = 0

    @property
    def background_color_hex(self) -> str | None:
        """배경색을 #RRGGBB 형식으로 반환합니다."""
        if self.background_color is None:
            return None
        # COLORREF는 0x00BBGGRR 형식
        r = self.background_color & 0xFF
        g = (self.background_color >> 8) & 0xFF
        b = (self.background_color >> 16) & 0xFF
        return f"#{r:02X}{g:02X}{b:02X}"

    @classmethod
    def from_record(cls, record: Record) -> "BorderFill":
        """Record에서 BorderFill을 파싱합니다.

        BORDER_FILL 구조 (HWP 5.0):
        - UINT16: 속성 (bit 0: 3D, bit 1: 그림자)
        - BORDER[4]: 4방향 테두리 (각 6 bytes: type(1) + width(1) + color(4))
        - BORDER: 대각선 (6 bytes)
        - UINT32: 채우기 유형 (0=없음, 1=색상, 2=그라데이션, 3=이미지)
        - 채우기 유형에 따라 추가 데이터 (색상: COLORREF 배경색 + COLORREF 무늬색)
        """
        data = record.data
        if len(data) < 2:
            return cls()

        offset = 0

        # 속성 (UINT16) - 일부 버전에서는 UINT32일 수 있음
        # 대부분의 HWP는 처음 4바이트가 attrs로 사용됨
        attrs = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        three_d = bool(attrs & 0x01)
        shadow = bool(attrs & 0x02)

        # 테두리 정보 파싱 (각 6바이트: type(1) + width(1) + color(4))
        # 4방향: 왼쪽, 오른쪽, 위, 아래 + 대각선
        border_types = [0, 0, 0, 0]
        for i in range(5):  # 4방향 + 대각선
            if len(data) >= offset + 6:
                if i < 4:
                    border_types[i] = struct.unpack_from("<B", data, offset)[0]
                offset += 6  # type(1) + width(1) + color(4)
            else:
                break

        # 채우기 유형 (UINT32)
        background_color = None
        if len(data) >= offset + 4:
            fill_type = struct.unpack_from("<I", data, offset)[0]
            offset += 4

            if fill_type == 1:  # 단색 채우기
                # 배경색 (COLORREF)
                if len(data) >= offset + 4:
                    background_color = struct.unpack_from("<I", data, offset)[0]
                    # 0xFFFFFFFF (white) or 0x00000000 (black) 체크
                    # 흰색은 None으로 처리하지 않음 (유효한 색상)

        return cls(
            background_color=background_color,
            three_d=three_d,
            shadow=shadow,
            left_border_type=border_types[0],
            right_border_type=border_types[1],
            top_border_type=border_types[2],
            bottom_border_type=border_types[3],
        )


@dataclass
class FaceName:
    """글꼴 정보."""

    name: str = ""  # 글꼴 이름
    alt_name: str = ""  # 대체 글꼴 이름
    default_name: str = ""  # 기본 글꼴 이름
    font_type: int = 0  # 글꼴 유형 (0=알 수 없음, 1=TTF, 2=HFT)

    @classmethod
    def from_record(cls, record: Record) -> "FaceName":
        """Record에서 FaceName을 파싱합니다."""
        data = record.data
        if len(data) < 3:
            raise ValueError(f"FaceName 데이터가 너무 짧습니다: {len(data)} bytes")

        offset = 0

        # 속성 (BYTE)
        attrs = struct.unpack_from("<B", data, offset)[0]
        offset += 1

        has_alt_font = bool(attrs & 0x80)
        has_font_type_info = bool(attrs & 0x40)
        has_default_font = bool(attrs & 0x20)

        # 글꼴 이름 길이 (WORD)
        name_len = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 글꼴 이름 (WCHAR array)
        name_bytes = data[offset : offset + name_len * 2]
        name = name_bytes.decode("utf-16-le", errors="replace").rstrip("\x00")
        offset += name_len * 2

        alt_name = ""
        font_type = 0
        default_name = ""

        # 대체 글꼴
        if has_alt_font and offset + 3 <= len(data):
            # 대체 글꼴 유형 (BYTE)
            font_type = struct.unpack_from("<B", data, offset)[0]
            offset += 1

            # 대체 글꼴 이름 길이 (WORD)
            alt_len = struct.unpack_from("<H", data, offset)[0]
            offset += 2

            # 대체 글꼴 이름
            if offset + alt_len * 2 <= len(data):
                alt_bytes = data[offset : offset + alt_len * 2]
                alt_name = alt_bytes.decode("utf-16-le", errors="replace").rstrip(
                    "\x00"
                )
                offset += alt_len * 2

        # 글꼴 유형 정보 (10 bytes)
        if has_font_type_info and offset + 10 <= len(data):
            offset += 10

        # 기본 글꼴
        if has_default_font and offset + 2 <= len(data):
            # 기본 글꼴 이름 길이 (WORD)
            default_len = struct.unpack_from("<H", data, offset)[0]
            offset += 2

            # 기본 글꼴 이름
            if offset + default_len * 2 <= len(data):
                default_bytes = data[offset : offset + default_len * 2]
                default_name = default_bytes.decode(
                    "utf-16-le", errors="replace"
                ).rstrip("\x00")

        return cls(
            name=name,
            alt_name=alt_name,
            default_name=default_name,
            font_type=font_type,
        )


class DocInfoParser:
    """DocInfo 스트림 파서.

    사용법:
        parser = DocInfoParser()
        parser.parse(docinfo_data)

        # ID로 조회
        char_shape = parser.get_char_shape(0)
        para_shape = parser.get_para_shape(0)
        face_name = parser.get_face_name(0)
    """

    def __init__(self) -> None:
        """DocInfoParser를 초기화합니다."""
        self._record_parser = RecordParser()
        self._char_shapes: list[CharShape] = []
        self._para_shapes: list[ParaShape] = []
        self._face_names: list[FaceName] = []
        self._border_fills: list[BorderFill] = []
        self._records: list[Record] = []

    def parse(self, data: bytes) -> None:
        """DocInfo 데이터를 파싱합니다.

        Args:
            data: DocInfo 스트림 데이터 (압축되었을 수 있음)
        """
        self._records = self._record_parser.parse_records(data, decompress=True)

        for record in self._records:
            try:
                if record.tag_id == HwpTagId.CHAR_SHAPE:
                    self._char_shapes.append(CharShape.from_record(record))
                elif record.tag_id == HwpTagId.PARA_SHAPE:
                    self._para_shapes.append(ParaShape.from_record(record))
                elif record.tag_id == HwpTagId.FACE_NAME:
                    self._face_names.append(FaceName.from_record(record))
                elif record.tag_id == HwpTagId.BORDER_FILL:
                    self._border_fills.append(BorderFill.from_record(record))
            except (ValueError, struct.error):
                # 파싱 실패한 레코드는 건너뜀
                continue

    def get_char_shape(self, shape_id: int) -> CharShape | None:
        """ID로 CharShape를 가져옵니다.

        Args:
            shape_id: CharShape ID (0부터 시작)

        Returns:
            CharShape 객체 또는 None
        """
        if 0 <= shape_id < len(self._char_shapes):
            return self._char_shapes[shape_id]
        return None

    def get_para_shape(self, shape_id: int) -> ParaShape | None:
        """ID로 ParaShape를 가져옵니다.

        Args:
            shape_id: ParaShape ID (0부터 시작)

        Returns:
            ParaShape 객체 또는 None
        """
        if 0 <= shape_id < len(self._para_shapes):
            return self._para_shapes[shape_id]
        return None

    def get_face_name(self, face_id: int) -> FaceName | None:
        """ID로 FaceName을 가져옵니다.

        Args:
            face_id: FaceName ID (0부터 시작)

        Returns:
            FaceName 객체 또는 None
        """
        if 0 <= face_id < len(self._face_names):
            return self._face_names[face_id]
        return None

    def get_border_fill(self, fill_id: int) -> BorderFill | None:
        """ID로 BorderFill을 가져옵니다.

        Args:
            fill_id: BorderFill ID (1부터 시작, HWP는 1-indexed)

        Returns:
            BorderFill 객체 또는 None
        """
        # HWP에서 border_fill_id는 1부터 시작하므로 인덱스로 변환
        idx = fill_id - 1
        if 0 <= idx < len(self._border_fills):
            return self._border_fills[idx]
        return None

    @property
    def char_shapes(self) -> list[CharShape]:
        """모든 CharShape 목록을 반환합니다."""
        return self._char_shapes

    @property
    def para_shapes(self) -> list[ParaShape]:
        """모든 ParaShape 목록을 반환합니다."""
        return self._para_shapes

    @property
    def face_names(self) -> list[FaceName]:
        """모든 FaceName 목록을 반환합니다."""
        return self._face_names

    @property
    def border_fills(self) -> list[BorderFill]:
        """모든 BorderFill 목록을 반환합니다."""
        return self._border_fills

    @property
    def records(self) -> list[Record]:
        """파싱된 모든 레코드를 반환합니다."""
        return self._records
