# Example: Quick Bug Fix

For small, focused changes — skip the planning phase entirely.

## When to use this

- Single-file bug fixes
- UI tweaks
- Config changes
- Anything you can describe in one sentence

## Example

> **You:** "모바일에서 로그인 버튼 클릭이 안 돼"

```
/c4-quick "fix login button not responding on mobile"

  ● Task T-011-0 created
    DoD: touch event handler added, tested on viewport <768px

  ◆ [worker] implementing fix...
  ✓ submitted  →  review passed  →  done

  Changed: src/components/LoginButton.tsx (+3 -1)
```

Done. One task, one worker, no ceremony.

## Difference from /c4-plan

| | `/c4-quick` | `/c4-plan` |
|---|---|---|
| Tasks | 1 (you describe it) | Multiple (CQ breaks it down) |
| Discovery | None | Yes — clarifying questions |
| Design | None | Architecture decisions |
| Best for | Clear, small changes | New features, refactors |

## Another example

> **You:** "API 응답에서 null 체크 빠진 것 같아, UserProfile 컴포넌트"

```
/c4-quick "add null guard for API response in UserProfile"

  ● Task T-012-0 created
    DoD: null/undefined handled, no console errors on empty profile

  ◆ [worker] implementing...
  ✓ done

  Changed: src/components/UserProfile.tsx (+5 -2)
```
