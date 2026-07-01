package graph

// Verbatim-body storage addressing (ADR-062 hydration contract). The producers
// (ast-source, doc-source) offload each entity's verbatim body to BodyStoreBucket
// and stamp BodyStoreInstance into the entity's body-handle triples; the fusion
// gateway (code-context) maps BodyStoreInstance back to the same store and
// dereferences the handle. Producer and resolver MUST agree on both — a mismatch
// yields a (nil, nil) body with NO error (the resolver just can't find the
// instance) — so they live here once rather than as copies across packages.
const (
	// BodyStoreInstance is the storage COMPONENT instance name stamped into
	// StorageReference.StorageInstance (gh#400) and mapped by the resolver. It
	// must also match run.go's "objectstore" storage component name.
	BodyStoreInstance = "objectstore"

	// BodyStoreBucket is the JetStream ObjectStore bucket verbatim bodies live in.
	BodyStoreBucket = "CONTENT"
)
