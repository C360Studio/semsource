# asset-ingestion-guards

## ADDED Requirements

### Requirement: Unminified twins of minified assets are indexed as files
The AST source SHALL treat a `X.js` file as a vendored asset, represented by its file entity only with no symbol entities, when a sibling `X.min.js` exists in the same directory. A watch path that sets `index_vendored_js: true` MUST disable this rule for its own files.

#### Scenario: A vendored twin is file-entity-only
- **GIVEN** a watch path containing `js/jquery.min.js`, `js/jquery.js` and `js/process_flow.js`
- **WHEN** the initial seed indexes the path
- **THEN** the graph contains file entities for all three files
- **AND** contains zero symbol entities whose source location is `js/jquery.js` or `js/jquery.min.js`
- **AND** contains the symbol entities of `js/process_flow.js`

#### Scenario: An operator opts back in
- **GIVEN** the same watch path with `index_vendored_js: true`
- **WHEN** the initial seed indexes the path
- **THEN** `js/jquery.js` yields its symbol entities
