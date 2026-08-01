## Context

See `proposal.md` — Why for the measurement. This records the two decisions that carry risk.

The splitter is `splitPassages(content []byte)` (`handler/doc/splitter.go:88`), and heading
detection is a `'#'` counter at `:237`. The sole production call site is
`handler/doc/entities.go:298`, one line below `mime := mimeForExt(filepath.Ext(path))` — so the
caller already knows the format and simply does not pass it.

## Goals / Non-Goals

**Goals:**

- AsciiDoc passages carry the heading and ancestry their markdown equivalents already do.
- Markdown behavior byte-identical, proven by the existing golden fixtures.
- Measured on the same instrument as the markdown fix, so the results are comparable.

**Non-Goals:**

- Interpreting AsciiDoc beyond section titles (attributes, block titles, includes, conditionals).
- Any change to size bounds, fence isolation, or merge rules.
- A general markup abstraction. Two formats do not justify a plugin layer.

## Decisions

### D1 — Format is threaded from the caller, never sniffed

`splitPassages` takes the document format as a parameter.

*Why not sniff the bytes.* Markdown's setext headings underline text with `=`:

```
Connected Systems API
=====================
```

AsciiDoc's section titles prefix with the same character (`= Title`). A sniffer keyed on `=` can
misclassify a setext markdown document as AsciiDoc and then mis-detect its headings — turning a
working corpus into a broken one to fix a different one. The prefix-versus-underline distinction is
learnable, but there is no reason to learn it: the caller has the extension.

*Cost.* An internal signature change. `splitPassages` is unexported and has one production call
site; the exported test seam `SplitPassagesBounded` gains the same parameter.

### D2 — AsciiDoc levels map onto the markdown ladder, not onto AsciiDoc's own numbering

AsciiDoc calls `= Title` the document title (level 0) and `== Section` level 1. Markdown calls
`# Title` level 1.

Mapping by **count of leading marker characters** — `=` → 1, `==` → 2, matching `#` → 1, `##` → 2 —
makes the two ladders coincide, so equivalent documents produce identical ancestry. Mapping by
AsciiDoc's own numbering would shift every AsciiDoc passage one level relative to its markdown
twin, and the ancestry chain would differ for content that is the same.

This is a deliberate departure from AsciiDoc's semantics in favour of cross-format consistency,
because ancestry is identity text here rather than a rendering instruction.

### D3 — The markdown golden fixtures are the regression gate

`handler/doc/testdata/dilution/*.golden` pin the measured markdown result. They must not move. If a
fixture changes, the change is wrong regardless of what it does for AsciiDoc: it would trade a
measured result for an unmeasured one.

## Risks / Trade-offs

- **A setext markdown document is misread as AsciiDoc** → Prevented structurally by D1: format comes
  from the extension, so `.md` never takes the AsciiDoc path.
- **AsciiDoc documents that open with attribute lines or comments before the title** → The title is
  found wherever it appears; nothing requires it on line one. Content above the first heading is
  already a handled case in the existing splitter.
- **A `=`-prefixed line inside a fenced or literal block is read as a heading** → Fence isolation
  already exists for markdown and operates on the same line scan; the AsciiDoc path must reuse it
  rather than pre-scan the raw bytes. Covered by a test.
- **Level ladders diverge for deeply nested AsciiDoc** → D2 maps by marker count, so `=====` and
  `#####` agree at every depth.

## Migration Plan

No migration. Passage boundaries are unchanged; AsciiDoc passages gain heading fields they should
already have carried. Because passage identity derives from heading ancestry, existing AsciiDoc
passage entities WILL take new IDs and titles on reindex — the same reindex the archived
`generalize-passage-dilution-split` change already required for markdown, and the supersession path
handles it.

## Open Questions

- Whether `.txt` should attempt any heading inference. Today it produces anonymous passages, which is
  honest for a format with no heading syntax. Deferred: no requirement here depends on it.
