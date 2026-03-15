# Interview Question Templates (AskUserQuestion)

## Area 1: Core Function Deep Dive

```python
AskUserQuestion(questions=[{
    "question": "What's the **most important success metric** for this feature?",
    "header": "Success Metrics",
    "options": [
        {"label": "Speed (response time < Xms)", "description": "Performance focus"},
        {"label": "Accuracy (error rate < X%)", "description": "Quality focus"},
        {"label": "Throughput (concurrent requests)", "description": "Scalability focus"},
        {"label": "Usability (minimize clicks)", "description": "UX focus"},
        {"label": "Other (explain)", "description": ""}
    ],
    "multiSelect": True
}])
```

## Area 2: Edge Cases Discovery

```python
AskUserQuestion(questions=[{
    "question": "How should the system behave in these situations?",
    "header": "Edge Cases",
    "options": [
        {"label": "Network drops mid-operation", "description": "Offline handling"},
        {"label": "Concurrent edits to same data", "description": "Conflict resolution"},
        {"label": "Data volume 100x expected", "description": "Scalability"},
        {"label": "Admin requests rollback", "description": "Recovery"},
        {"label": "Never considered (define now)", "description": ""}
    ],
    "multiSelect": True
}])
```

## Area 3: Failure Scenarios

```python
AskUserQuestion(questions=[{
    "question": "When this feature **fails**, what user experience should we provide?",
    "header": "Failure UX",
    "options": [
        {"label": "Auto-retry then notify", "description": "System attempts fix"},
        {"label": "Immediate error message", "description": "Transparent feedback"},
        {"label": "Offer fallback path", "description": "Alternative option"},
        {"label": "Silent logging only", "description": "User unaware"},
        {"label": "Explain directly", "description": ""}
    ],
    "multiSelect": False
}])
```

## Area 4: Tradeoffs

```python
AskUserQuestion(questions=[
    {
        "question": "Which is **non-negotiable**?",
        "header": "Non-negotiables",
        "options": [
            {"label": "Fast launch", "description": "MVP fast → reduce features"},
            {"label": "Complete features", "description": "All cases → take longer"},
            {"label": "Best performance", "description": "Optimize → increase complexity"},
            {"label": "Simple code", "description": "Maintainability → feature constraints"}
        ],
        "multiSelect": False
    },
    {
        "question": "What can you **sacrifice** for the above choice?",
        "header": "Tradeoffs",
        "options": [
            {"label": "Launch timeline", "description": "Can delay"},
            {"label": "Feature scope", "description": "Can exclude some"},
            {"label": "Performance", "description": "Good enough is fine"},
            {"label": "Code quality", "description": "Refactor later"}
        ],
        "multiSelect": True
    }
])
```

## Area 5: Hidden Assumptions

Open-ended questions:
- "How **technically skilled** are users of this feature?"
- "How **sensitive** is the data? (PII, payment info, etc.)"
- "Must this feature **integrate with other systems**?"
- "Will it be used on **mobile**?"
- "Do you need **multi-language/timezone** support?"
