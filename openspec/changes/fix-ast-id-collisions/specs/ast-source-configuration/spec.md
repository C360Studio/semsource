# ast-source-configuration

## ADDED Requirements

### Requirement: Vendored and minified JavaScript is excluded by default
The AST source SHALL skip `*.min.js` and `*.min.mjs` files, and any `X.js` file with a sibling `X.min.js`, unless the watch path sets `include_vendored: true`.

#### Scenario: Minified bundle and its unminified twin are skipped
- **WHEN** a watch path contains `js/jquery.min.js`, `js/jquery.js` and `js/process_flow.js`
- **THEN** only `js/process_flow.js` is parsed

#### Scenario: An operator opts back in
- **WHEN** the same watch path sets `include_vendored: true`
- **THEN** `js/jquery.min.js`, `js/jquery.js` and `js/process_flow.js` are all parsed
