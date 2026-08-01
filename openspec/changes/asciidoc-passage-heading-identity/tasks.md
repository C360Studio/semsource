# Tasks

## 1. Make heading recognition format-aware

- [ ] 1.1 Introduce a document-format value in `handler/doc` (markdown, asciidoc, plain) resolved
      from the extension alongside the existing `mimeForExt`.
- [ ] 1.2 Thread it into `splitPassages` / `splitPassagesBounded` and update the exported test seam
      `SplitPassagesBounded` in `export_test.go`.
- [ ] 1.3 Generalize the `'#'` counter at `splitter.go:237` to the format's marker character,
      mapping by **count** so `==` and `##` are the same level (design — D2).
- [ ] 1.4 Apply the same treatment to the title path at `handler.go:103`
      (`strings.HasPrefix(line, "# ")`).
- [ ] 1.5 Pass the format at the call site `entities.go:298` — `mime` is already resolved one line
      above.

## 2. Prove markdown did not move

- [ ] 2.1 `handler/doc/testdata/dilution/*.golden` must be byte-identical. A moved fixture means the
      change is wrong, not that the fixture needs regenerating.
- [ ] 2.2 Existing splitter, isolation, and dilution tests green with no edits to their expectations.
- [ ] 2.3 Test that a **setext** markdown document (`Title` underlined with `===`) still splits as
      markdown — the misclassification D1 exists to prevent.

## 3. Prove AsciiDoc gained identity

- [ ] 3.1 Table test over equivalent markdown/AsciiDoc documents: same headings, same ancestry.
      This is the cross-format agreement requirement.
- [ ] 3.2 Test nesting to at least three levels, asserting `HeadingPath` ordering outermost-first.
- [ ] 3.3 Test that a `=`-prefixed line inside a fenced/literal block is not treated as a heading.
- [ ] 3.4 Test that a plain-text document still produces passages with no heading and no error.

## 4. Measure it the way markdown was measured

- [ ] 4.1 Add an AsciiDoc corpus fixture to the offline dilution harness.
- [ ] 4.2 Record before/after on that corpus, so the AsciiDoc result is comparable to the
      markdown 22/22 rather than merely asserted.
- [ ] 4.3 Update `docs/testing/tier-baselines.md` with the AsciiDoc numbers if the corpus is one an
      operator would recognize.

## 5. Gates

- [ ] 5.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`,
      `go test -tags=integration ./...` green.
- [ ] 5.2 `openspec validate asciidoc-passage-heading-identity --strict` green.
- [ ] 5.3 Boot a real stack over an AsciiDoc corpus and confirm through MCP that passages carry
      section titles — the gap was invisible to every existing test, so a unit test alone does not
      close it.
