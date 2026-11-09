## ADDED Requirements

### Requirement: The fusion graph client maps onto graph query verbs

The `fusionnats` client SHALL serve the fusion engine's graph reads over the live `graph.query.*` verbs. Its
status read SHALL succeed against a ready graph. A batch entity read SHALL return each requested entity. A
single-entity read of an absent ID SHALL return no entity and no error. A relationship read SHALL return outgoing
edges with the target as the related entity and incoming edges with the source as the related entity. A prefix
resolve SHALL return the IDs of the ingested entities under that prefix.

#### Scenario: Reads against an ingested caller and callee

- **GIVEN** a caller and a callee entity ingested through graph-ingest, with a call edge between them
- **WHEN** the client reads status, both entities in a batch, an absent ID, both edge directions, and the prefix
- **THEN** status succeeds, the batch returns both entities, the absent read returns nothing without error, the
  outgoing edge names the callee, the incoming edge names the caller, and the prefix resolve includes both IDs

### Requirement: Code-lens fusion hydrates verbatim bodies from the content store

A code entity whose body was offloaded SHALL carry body-store and body-key handle triples. Fusion requests that
want bodies SHALL dereference those handles over the same content store the producer wrote and return the body
verbatim, alongside the entity's relations, its stamped class, and deterministic provenance.

#### Scenario: A handle written directly is hydrated

- **GIVEN** a function entity whose body is stored under a key and referenced by handle triples
- **WHEN** the code lens fuses it, wanting body and relations
- **THEN** the node's body equals the stored bytes, its relations name the callee, its class is the stamped
  software-code class, and the provenance is deterministic

#### Scenario: A body offloaded by ast-source is hydrated

- **GIVEN** ast-source indexing a real Go tree, offloading bodies to the content store
- **WHEN** a fusion request resolves a function by name and wants its body
- **THEN** the returned body is the function's verbatim source, not a placeholder
