# Review Document Structure

## Korean Structure

```
[인사말 — 리뷰어 역할 맡게되어 영광]
[논문 요약 + 긍정 마무리]
[전환구]

A. 주요 의견 (Major Comments)
  1. (optional) 동기/contribution 메타 코멘트
  2~. 기술 이슈 (질문형으로)

B. 보조 의견 (Minor Comments)
  (번호 매김, 간결하게 1-2줄)

C. 그 밖에 (선택적 — Regular Paper면 유지, Letter면 생략 가능)

[개인 의견 + 마무리]
감사합니다.

---
에디터에게
[판정 근거 + 추천]
감사합니다.
```

## English Structure

```
[Opening — honor to serve as reviewer]
[Paper summary + positive note]
[Transition]

A. Major Comments
B. Minor Comments
C. Additional Remarks (optional)

[Closing]
Thank you.

---
To the Editor:
[Assessment + recommendation]
Thank you.
```

## Output Files

1. `review/.draft.md` — AI-generated original (for persona learning)
2. `review/리뷰의견.md` — Initial version (same as draft initially)

Both contain full Korean + English review.
