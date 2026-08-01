# Tasks

## 1. Make heading recognition format-aware

- [x] 1.1 Introduce a document-format value in `handler/doc` (markdown, asciidoc, plain) resolved
      from the extension alongside the existing `mimeForExt`.
- [x] 1.2 Thread it into `splitPassages` / `splitPassagesBounded` and update the exported test seam
      `SplitPassagesBounded` in `export_test.go`.
- [x] 1.3 Generalize the `'#'` counter at `splitter.go:237` to the format's marker character,
      mapping by **count** so `==` and `##` are the same level (design — D2).
- [x] 1.4 Apply the same treatment to the title path at `handler.go:103`
      (`strings.HasPrefix(line, "# ")`).
- [x] 1.5 Pass the format at the call site `entities.go:298` — `mime` is already resolved one line
      above.

## 2. Prove markdown did not move

- [x] 2.1 `handler/doc/testdata/dilution/*.golden` must be byte-identical. A moved fixture means the
      change is wrong, not that the fixture needs regenerating.
- [x] 2.2 Existing splitter, isolation, and dilution tests green with no edits to their expectations.
- [x] 2.3 Test that a **setext** markdown document (`Title` underlined with `===`) still splits as
      markdown — the misclassification D1 exists to prevent.

## 3. Prove AsciiDoc gained identity

- [x] 3.1 Table test over equivalent markdown/AsciiDoc documents: same headings, same ancestry.
      This is the cross-format agreement requirement.
- [x] 3.2 Test nesting to at least three levels, asserting `HeadingPath` ordering outermost-first.
- [x] 3.3 Test that a `=`-prefixed line inside a fenced/literal block is not treated as a heading.
- [x] 3.4 Test that a plain-text document still produces passages with no heading and no error.

## 4. Measure it the way markdown was measured

- [x] 4.1 Measured against the **real** OGC Connected Systems API specification (798 `.adoc` files,
      2,126 passages) rather than a synthetic fixture — the corpus the A/B actually uses.
- [x] 4.2 Before/after recorded in `proposal.md`. The real corpus reframed the defect: today's
      behavior does not *miss* headings, it **invents** them. Only 5 lines in the corpus start with
      `#`, so the 1,601 "headings" came from setext detection reading AsciiDoc's 705 `----` listing
      delimiters as H2 underlines. Passage count 2,126 → 1,794 and raw heading count 1,601 → 1,020
      both fall as those vanish, while the signals that matter rise: distinct ancestries 435 → 618,
      max nesting depth 2 → 4.
- [ ] 4.3 Update `docs/testing/tier-baselines.md` once an AsciiDoc corpus joins the tier baselines —
      deferred with the multi-repo measurement it belongs beside.

## 5. Gates

- [x] 5.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`,
      `go test -tags=integration ./...` green.
- [x] 5.2 `openspec validate asciidoc-passage-heading-identity --strict` green.
- [x] 5.3 Booted a tier-1 compose stack over 173 `.adoc` files of the real OGC Connected Systems
      API spec (868 entities) and queried through MCP. Confirmed end to end:
      - Section ancestry reaches passage identity — `clause_0_front_material § Abstract`, and
        multi-level `clause_13_requirements_class_sampling_features § Requirements Class "Sampling
        Features" § Dynamic properties`.
      - A document title from `= ` is extracted rather than falling back to the filename stem —
        `Standard template in Metanorma`.
      - Passages above the first heading still name themselves positionally
        (`clause_0_front_material § passage 1`), the documented headingless behavior.
      Note `graph_search` labels these as entity-ID instance segments rather than titles: those
      labels come from the substrate's digest fallback, not from passage identity, and are unrelated
      to this change.

## 6. Follow-up (not this change)

- [ ] 6.1 Teach `scanLines` AsciiDoc's `----` / `....` block delimiters. Measured residue: 7 of
      1,064 `=` headings sit inside such a block (0.7%). Deferred because `scanLines` is shared with
      markdown and the markdown golden fixtures are this change's regression gate — a 0.7%
      correction does not justify putting a measured result at risk in the same change.
