# Scorecard A/B — doc passage chunking

Corpus held fixed (`git archive` of 253f9fd: 206 md, 326 go, no node_modules, no
.git); only the semsource binary varies. questions.json version 1.

| run | score | entities | median top-node body | mean total body | doc answers led by an empty node |
|---|---|---|---|---|---|
| pre-chunking | 19/20 | 4786 | 12,142 B | 45,860 B | 0/11 |
| post-chunking | 20/20 | 5312 | 1,102 B | 15,780 B | 5/11 |

## Reading it

**The score delta is weak evidence and should not be led with.** Fact-presence
grading cannot separate whole-file retrieval from passage retrieval: a 31 KB README
body trivially contains every fact in it, so a substring matcher passes whether the
system found the right paragraph or dumped the document. Only D06 flipped.

**Evidence precision is the real result.** The median top-ranked doc body fell from
12,142 B to 1,102 B — an answer about a single port shipped 31 KB of prose before and
about 1 KB after. Mean total body per answer fell 2.9x even though the answer now
carries 20 nodes instead of 4: more pieces, each far smaller and individually
addressable.

No band regressed. doc-early (the control) held at 3/3, so the late-band gain was not
bought with an early-band loss. Both negative questions passed on both sides — zero
fabrication throughout.

Corpus cost: +526 entities (+11%).

## Defect this measurement found

**Body-less parent documents rank above their own passages.** 5 of 11 doc answers now
lead with a `kind: document` node whose body is empty — a citation with nothing in it —
where pre-chunking that never happened (0/11). Confirmed live:

```
0    document  Point Claude Code (or any agent) at SemSource over MCP
0    document  SemSource
433  passage   ...MCP § Point Claude Code (...)
609  passage   CLAUDE.md § CLAUDE.md
```

This violates the requirement in the change's own `retrieval-ranking` delta: *"A parent
document entity carrying a title but no body SHALL NOT displace passage entities that
carry actual content."* It is the empty-bodied-title-node pattern the 2026-07-19 audit
found with config entities, reproduced by this design. Task 7.1 existed to check exactly
this; it is unmet.

Secondary, cosmetic: when a document's H1 equals its title, the qualified passage title
duplicates it ("CLAUDE.md § CLAUDE.md").
