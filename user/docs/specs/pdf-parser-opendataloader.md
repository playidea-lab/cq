feature: PDF Parser OpenDataLoader 교체
domain: web-backend
requirements:
  - "[SHALL] PdfParser는 opendataloader-pdf SDK를 사용하여 PDF를 파싱한다"
  - "[SHALL] 출력은 기존 ir_models.Document(HeadingBlock, ParagraphBlock, TableBlock, ImageBlock, ListBlock)와 동일한 IR 구조를 유지한다"
  - "[SHALL] parse()와 parse_with_images() 두 메서드 모두 동작한다"
  - "[SHALL] OCR 모드를 지원하며 한국어/영어를 기본 언어로 한다"
  - "[SHOULD] 하이브리드 모드로 복잡 테이블/수식 추출을 지원한다"
  - "[SHOULD] 기존 smoke test (test_c2_parsers_smoke.py)를 모두 통과한다"
  - "[UNWANTED] PyMuPDF(fitz) 의존성은 pdf_parser.py에서만 제거, review/converter.py는 유지"
  - "[WILL NOT] 다른 파서(DOCX, HWP 등)는 변경하지 않는다"
non_functional:
  - "CPU-only 동작 (GPU 불필요)"
  - "Java 11+ 런타임 필요"
out_of_scope:
  - "PDF/UA 접근성 기능"
  - "하이브리드 AI 백엔드 커스터마이징"
  - "review/converter.py의 PyMuPDF 사용"
