# 6-Axis Detailed Analysis

## Dimensions and Weights

1. **Quality of Subject** (1.0) — motivation, significance, relevance
2. **Novelty / Originality** (1.0) — new approach, contribution scope
3. **Technical Soundness** (1.5) — assumptions, derivations, edge cases
4. **Experimental Validation** (1.5) — baselines, conditions, statistics
5. **Discussion & Completeness** (1.0) — interpretation, limitations, future
6. **Presentation Quality** (0.8) — flow, figures, formatting

For each dimension: evaluate checklist items, assign score (1-10), note specific issues with equation/figure/page references.

## Math Verification

For papers with mathematical derivations:
- Verify key equations step-by-step
- Check limiting cases
- If numerical verification needed, create `artifacts/` scripts
- Save to `review/math_verification.md`

## Overall Assessment

### Weighted Score

```
score = Σ(dimension_score × weight) / Σ(weight)
```

### Recommendation

| Score | Decision |
|-------|----------|
| 1-3 | Reject |
| 4-5 | Major Revision |
| 6-7 | Minor Revision |
| 8-10 | Accept |

### Issue Classification

- **Major Comments**: Methodology flaws, missing evidence, incorrect derivations
- **Minor Comments**: Presentation, missing details, formatting

### Claim-Evidence Mapping

For each major claim: map to evidence and assess strength.
