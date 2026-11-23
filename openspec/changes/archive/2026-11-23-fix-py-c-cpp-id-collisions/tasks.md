# One entity ID per Python, C and C++ declaration — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-23, sha256:57967321fe4e; line entered by claude at the owner's instruction). Owner rulings 2026-11-23 on #338 (comment 7115830103): C/C++ fold (definition beats prototype, else first in source); Python `-setter`/`-deleter`, `@overload` stubs dropped when implemented, last binding wins.

## 2. Implementation

- [x] 2.1 `ast.FoldDeclarations` with a unit test for replacement, position and `Contains` union.
- [x] 2.2 C parser folds with the definition policy.
- [x] 2.3 C++ parser folds with the definition policy.
- [x] 2.4 Python parser: accessor discriminator, stub marking, fold with the last-binding policy.
- [x] 2.5 Guard test covers python, c and cpp with fixtures for every class in #338's step 0 comment.

## 3. Verify

- [x] 3.1 The collision probe reports zero on llama.cpp (both `.h` routings; 2,673 and 2,659 before), the carnot site-packages tree (1,996 before) and semselect (5 before), 2026-11-23.
- [x] 3.2 `task lint`, `go test ./...` and `go test -race ./source/ast/...` pass.
- [x] 3.3 Independent review: `graph-event-reviewer` approved at `2d91a30` after W1 (decorator trailing comment) was fixed; owner ruled W2 (no prototype doc carry-over) on #338. The gate review runs through `ateam review request`.
