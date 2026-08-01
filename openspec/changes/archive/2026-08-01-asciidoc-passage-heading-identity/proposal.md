## Why

The passage splitter detects headings by counting `'#'` (`handler/doc/splitter.go:237`) — markdown
ATX only. AsciiDoc uses `=` / `==` / `===`, so **no AsciiDoc heading is ever recognized**.

Measured two ways. On identical synthetic content with only the heading syntax changed:

| | passages | with heading |
| --- | --- | --- |
| AsciiDoc | 15 | **0** |
| Markdown | 15 | **15** |

And on the real OGC Connected Systems API specification — 798 `.adoc` files, 2,126 passages:

| | passages | with heading | distinct ancestries | max depth |
| --- | --- | --- | --- | --- |
| Markdown detection (today) | 2,126 | 1,601 (75.3%) | 435 | 2 |
| AsciiDoc detection | 1,794 | 1,020 (56.9%) | **618** | **4** |

The real corpus tells the sharper story: today's behavior is not *missing* headings, it is
**inventing wrong ones**. The corpus contains only 5 lines beginning with `#`, so almost none of
those 1,601 headings are ATX. They come from setext detection — AsciiDoc delimits listing blocks
with `----`, which is also a valid markdown H2 underline, so **every prose line above a code block
was mined as a heading**, 705 delimiters' worth.

So the passage count drops (spurious sections disappear), the raw heading count drops, and the
signals that matter both rise: distinct ancestries 435 → 618 and maximum nesting depth 2 → 4. The
identity text goes from mostly-fabricated to real section structure.

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
