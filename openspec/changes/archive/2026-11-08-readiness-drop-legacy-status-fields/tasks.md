# Tasks: readiness drops the legacy status fields

## 1. Contract

- [x] 1.1 Drop `Revision` / `LastSynced` from `workbenchIndexStatusWire` and `workbenchIndexReadiness`
- [x] 1.2 Regression test: a reply carrying the legacy trio yields none of it, numeric revisions intact
- [x] 1.3 UI capabilities contract and workbench stop reading and rendering both fields

## 2. Record

- [x] 2.1 Spec delta on `ingestion-readiness`
- [x] 2.2 Note on #226 what ADR-109 adoption still requires at port time
