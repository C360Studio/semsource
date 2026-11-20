## REMOVED Requirements

### Requirement: SemStreams remains the shipped substrate

**Reason**: Main cut over to SemEngine at Tier 0 (#222, ADR-0013); the rule that mainline waits for
full semembed-backed workloads was superseded by the owner rulings on #237.

**Migration**: The cutover rule lives in `tier0-consumer-qualification` "Mainline cutover qualifies
Tier 0 and BM25 while deferring neural 04C". The frozen SETUP 03A baseline on SemStreams remains
recorded in this capability's Purpose and in "Migration comparisons use immutable inputs".
