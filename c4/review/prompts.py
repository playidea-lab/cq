"""
System prompts for Claude API interactions.
"""

STRUCTURE_ANALYSIS_PROMPT = """You are analyzing an academic paper to extract its structural information.

Please analyze the provided paper pages and extract the following information:

1. **Title and Authors**: Extract the paper title and list of authors.

2. **Sections**: List all section headings in order with their page numbers and levels.
   - Level 1: Main sections (e.g., "1. Introduction", "2. Methods")
   - Level 2: Subsections (e.g., "2.1 Data Collection")
   - Level 3: Sub-subsections (e.g., "2.1.1 Preprocessing")

3. **Figures**: List all figures with their captions, page numbers, and figure numbers.
   - Example: "Figure 1: System Architecture" on page 3

4. **Tables**: List all tables with their captions, page numbers, and table numbers.
   - Example: "Table 1: Experimental Results" on page 5

5. **Equations**: Count the total number of numbered equations.

6. **References**: Count the total number of references in the bibliography.

Return the information in the following JSON format:
```json
{
  "title": "Paper title here",
  "authors": ["Author One", "Author Two"],
  "sections": [
    {"title": "Introduction", "page_number": 1, "level": 1},
    {"title": "Related Work", "page_number": 2, "level": 1},
    {"title": "Background", "page_number": 2, "level": 2}
  ],
  "figures": [
    {"caption": "System architecture", "page_number": 3, "figure_number": "Figure 1"}
  ],
  "tables": [
    {"caption": "Experimental results", "page_number": 5, "table_number": "Table 1"}
  ],
  "equations_count": 5,
  "references_count": 25
}
```

Important guidelines:
- Be precise with page numbers (use 1-based indexing).
- Include only numbered equations in the count.
- For sections, capture the hierarchical structure accurately.
- If information is unclear or missing, make best-effort estimates but note uncertainties.
"""


def build_section_analysis_prompt(profile=None) -> str:
    """
    Build section analysis prompt, optionally incorporating reviewer profile.

    Args:
        profile: ReviewerProfile object or None

    Returns:
        Formatted prompt string
    """
    base_prompt = """You are conducting a detailed academic review of a paper.

Analyze each section according to these four perspectives:

1. **Technical Completeness**: Are technical details sufficient? Are methods well-explained?
2. **Academic Contribution**: Does the section contribute novel insights or results?
3. **Methodological Rigor**: Are methods sound and properly validated?
4. **Organization & Clarity**: Is the section well-structured and clearly written?

For each section, provide:
- **Section Name**: Name of the section
- **Score**: Overall quality score (1-10)
- **Strengths**: List of specific strengths
- **Weaknesses**: List of specific weaknesses
- **Specific Comments**: Detailed comments and suggestions for improvement

Return the information in JSON format:
```json
{
  "sections": [
    {
      "section_name": "Introduction",
      "score": 7,
      "strengths": ["Clear motivation", "Good background"],
      "weaknesses": ["Missing recent related work"],
      "specific_comments": ["Consider adding comparison with recent work by Smith et al. (2024)"]
    }
  ],
  "overall_notes": "General observations about section quality"
}
```
"""

    if profile:
        profile_section = f"""
**Reviewer Profile**:
- Expertise: {', '.join(profile.expertise_areas) if profile.expertise_areas else 'General'}
- Review Style: {profile.review_style}
- Focus on Novelty: {'Yes' if profile.focus_on_novelty else 'No'}
- Focus on Reproducibility: {'Yes' if profile.focus_on_reproducibility else 'No'}

Please adjust your review according to this profile.
"""
        return base_prompt + profile_section

    return base_prompt


def build_assessment_prompt(user_inputs=None) -> str:
    """
    Build overall assessment prompt, optionally incorporating user inputs.

    Args:
        user_inputs: List of user discussion points/questions or None

    Returns:
        Formatted prompt string
    """
    base_prompt = """You are providing an overall assessment of an academic paper based on section-level analysis.

Based on the section evaluations, provide a comprehensive overall assessment:

**Dimension Scores** (1-10 scale):
- novelty: How novel and original is the work?
- clarity: How clear and well-written is the paper?
- rigor: How rigorous and sound are the methods?
- significance: How significant is the contribution?
- reproducibility: Can the work be reproduced?

**Overall Score**: Weighted average of dimension scores (1-10)

**Recommendation**: Choose one:
- Accept: Ready for publication
- MinorRevision: Minor changes needed
- MajorRevision: Significant changes needed
- Reject: Not suitable for publication

**Summary**: 2-3 sentence overall summary

**Key Strengths**: List 3-5 main strengths

**Key Weaknesses**: List 3-5 main weaknesses

Return the information in JSON format:
```json
{
  "dimension_scores": {
    "novelty": 7.5,
    "clarity": 8.0,
    "rigor": 7.0,
    "significance": 8.5,
    "reproducibility": 6.5
  },
  "overall_score": 7.5,
  "recommendation": "MinorRevision",
  "summary": "This paper presents a novel approach to X with strong results. Some methodological details need clarification.",
  "key_strengths": [
    "Novel approach to problem X",
    "Comprehensive experiments",
    "Clear presentation"
  ],
  "key_weaknesses": [
    "Limited comparison with recent work",
    "Some parameters not justified",
    "Reproducibility concerns"
  ]
}
```
"""

    if user_inputs:
        user_section = "\n**User Discussion Points**:\n"
        for i, point in enumerate(user_inputs, 1):
            user_section += f"{i}. {point}\n"
        user_section += "\nPlease incorporate these points into your assessment.\n"
        return base_prompt + user_section

    return base_prompt


def build_review_generation_prompt(assessment, user_inputs=None, config=None) -> str:
    """
    Build review generation prompt.

    Args:
        assessment: OverallAssessment object
        user_inputs: Optional user discussion points
        config: Optional ReviewConfig

    Returns:
        Formatted prompt string
    """
    prompt = """당신은 학술 논문 심사자입니다. 다음 평가 결과를 바탕으로 리뷰의견.md 형식의 리뷰를 작성해주세요.

**평가 결과**:
"""

    # Add assessment details
    prompt += f"\n- 종합 점수: {assessment.overall_score}/10\n"
    prompt += f"- 추천: {assessment.recommendation}\n"
    prompt += f"- 요약: {assessment.summary}\n\n"

    prompt += "**주요 강점**:\n"
    for strength in assessment.key_strengths:
        prompt += f"- {strength}\n"

    prompt += "\n**주요 약점**:\n"
    for weakness in assessment.key_weaknesses:
        prompt += f"- {weakness}\n"

    if user_inputs:
        prompt += "\n**토론 포인트**:\n"
        for point in user_inputs:
            prompt += f"- {point}\n"

    prompt += """

다음 형식으로 리뷰를 작성해주세요:

1. **인사말** (간단한 감사 인사)
2. **논문 요약** (2-3문장으로 논문의 핵심 내용 요약)
3. **강점** (주요 강점 3-5개)
4. **번호 매긴 세부 코멘트** (1., 2., 3., ... 형식으로 구체적 개선 사항)
5. **일반 지적** (전반적인 개선 사항)
6. **종합 평가** (Accept/Minor Revision/Major Revision/Reject 및 이유)
7. **에디터 코멘트** (선택사항)

한국어로 작성하고, 전문적이고 건설적인 톤을 유지해주세요.
"""

    return prompt


def build_translation_prompt(korean_text: str) -> str:
    """
    Build translation prompt for Korean to English.

    Args:
        korean_text: Korean text to translate

    Returns:
        Formatted prompt string
    """
    return f"""Please translate the following Korean academic review to English.
Maintain the professional and constructive tone, and preserve the structure (headings, bullet points, numbering).

Korean text:
```
{korean_text}
```

Provide only the English translation, maintaining the same format and structure.
"""

