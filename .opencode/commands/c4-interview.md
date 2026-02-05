# C4 Interview - Deep Exploratory Requirements Interview

Deep exploratory interview to discover hidden requirements.

> Danny Postma's Rule: "Ask not obvious questions, very in-depth, and continually until I have complete clarity."

## Usage

```
/c4-interview user-authentication
/c4-interview payment-system
```

## Instructions

### Core Principles

1. **No Obvious Questions** - No "which language?" or "which database?"
2. **Very In-Depth** - Dig into edge cases, failure scenarios, tradeoffs
3. **Continually Until Complete** - Don't stop until clarity is achieved

### Interview Areas

1. **Core Function Deep Dive** - Success metrics, real problem
2. **Edge Cases Discovery** - What user hasn't thought about
3. **Failure Scenarios** - What happens when things go wrong
4. **Tradeoffs** - What's negotiable vs non-negotiable
5. **Hidden Assumptions** - Implicit requirements made explicit

### Good vs Bad Questions

```
Bad: "React or Vue?"
Good: "5,000 concurrent users hit this feature - what should happen?"

Bad: "Need tests?"
Good: "How will you know this works correctly without manually checking?"

Bad: "Use JWT?"
Good: "Session expired while writing a long post - how to handle?"
```

### Output

Create spec file at: `.c4/specs/{feature}/interview.md`

Include:
- Problem Statement (real problem, not feature list)
- Core Requirements (with success criteria)
- Discovered Edge Cases
- Failure Scenarios
- Tradeoff Decisions
- Hidden Requirements
- Performance/Security Considerations

### Integration

After interview:
1. Run `/c4-plan {feature}` to convert to EARS requirements
2. Or re-run `/c4-interview {feature}` to refine

## Notes

- This skill focuses on **discovering** requirements, not implementing
- Output integrates with C4's Discovery phase
- Results in structured spec for Design phase
