## Why

The passage splitter detects headings by counting `'#'` (`handler/doc/splitter.go:237`) — markdown
ATX only. AsciiDoc uses `=` / `==` / `===`, so **no AsciiDoc heading is ever recognized**.

Measured on identical content with only the heading syntax changed:

| | passages | with heading |
| --- | --- | --- |
| AsciiDoc | 15 | **0** |
| Markdown | 15 | **15** |

Passage *count* is unaffected — size-driven splitting still runs. What is lost is **identity**:
every `.adoc` passage carries `Heading: ""` and `HeadingPath: []`.

That matters because heading-path identity **is** the passage-dilution fix (archived
`generalize-passage-dilution-split`, 22/22 on both corpora). For AsciiDoc corpora that fix is
inert — they run the pre-fix behavior with anonymous passages.

`.adoc` support exists precisely for specification repositories. Commit `a92100b`: *"enabling
ingestion of standards repos and technical specs that use AsciiDoc (e.g. OGC API specs)"*, with the
warning *"AsciiDoc markup passes through as text"*. The OGC Connected Systems API is exactly such a
spec — docs-only, authored in AsciiDoc, implemented by adopters — and it is a primary reference
corpus for the planned agent A/B. Today every passage of it is anonymous.

## What Changes

- **The splitter learns AsciiDoc section titles.** `=`-prefixed lines produce the same `Heading` and
  `HeadingPath` that `#`-prefixed lines already do, so an AsciiDoc passage carries the same identity
  a markdown passage does.
- **Format comes from the caller, not from sniffing the bytes.** The document's MIME type is already
  resolved one line above the splitter call (`handler/doc/entities.go:297`), so the format is passed
  in. Markdown's setext headings (`Title` underlined with `===`) make byte-sniffing genuinely
  ambiguous; the caller has the unambiguous answer.
- **Markdown behavior is unchanged, and pinned.** The dilution golden fixtures must not move. A
  change that improved AsciiDoc while perturbing markdown would trade one measured result for an
  unmeasured one.
- **Measured the same way the markdown fix was.** The existing offline dilution harness is the
  instrument; AsciiDoc gets a corpus fixture so the result is comparable rather than asserted.

## Non-goals

- Rendering or interpreting AsciiDoc beyond section titles. Attribute entries (`:name: value`),
  block titles (`.Title`), includes (`include::`), and conditionals stay pass-through text, exactly
  as today. Only section titles gain meaning.
- Changing the passage size bounds, the fence-isolation rules, or any other part of the splitting
  algorithm.
- `.txt` and unknown extensions, which have no heading syntax and keep producing anonymous passages.
- Adding C/C++/Rust parsers, the other A/B corpus gap — unrelated code path, separate change.

## Consumers

Anyone retrieving from an AsciiDoc corpus: `doc_context`, `code_search`'s doc lens, `graph_search`,
and the GraphQL/HTTP doc routes all read passage identity. The agent A/B against the Connected
Systems API specification is the motivating consumer.

## Capabilities

### Modified Capabilities

- `doc-passage-chunking`: passage identity becomes format-aware — the heading contract holds for
  every document format whose headings the corpus can express, not only markdown.

## Impact

- `handler/doc/splitter.go` — heading recognition, parameterized by format.
- `handler/doc/entities.go:298` — pass the already-resolved format to the splitter.
- `handler/doc/handler.go:103` — the title path, which has the same markdown-only assumption.
- `handler/doc/testdata/` — an AsciiDoc corpus fixture for the dilution harness.
- No change to entity IDs, predicates, config, or any query contract: the same passages are
  produced, and they gain the heading fields they should already have carried.
