# One entity ID per Python, C and C++ declaration — tasks

## 1. Approval

- [x] 1.1 Owner rulings 2026-11-23 on #338 (comment 7115830103): C/C++ fold (definition beats prototype, else first in source); Python `-setter`/`-deleter`, `@overload` stubs dropped when implemented, last binding wins.

## 2. Implementation

- [ ] 2.1 `ast.FoldDeclarations` with a unit test for replacement, position and `Contains` union.
- [ ] 2.2 C parser folds with the definition policy.
- [ ] 2.3 C++ parser folds with the definition policy.
- [ ] 2.4 Python parser: accessor discriminator, stub marking, fold with the last-binding policy.
- [ ] 2.5 Guard test covers python, c and cpp with fixtures for every class in #338's step 0 comment.

## 3. Verify

- [ ] 3.1 The collision probe reports zero on llama.cpp (both `.h` routings) and the site-packages tree.
- [ ] 3.2 `task lint`, `go test ./...` and `go test -race ./source/ast/...` pass.
- [ ] 3.3 Independent review (`graph-event-reviewer`).
