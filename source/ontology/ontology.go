// Package ontology aligns semsource entity kinds and predicates to the BFO/CCO
// upper ontologies for standards-aligned ("standards at work") ranking. The IRI
// constants live in semstreams (vocabulary/bfo, vocabulary/cco); this package
// owns only the semsource-specific class mapping and a minimal subclass table
// (hierarchy.go) used to rank fused results.
//
// It is alignment, not reasoning: no OWL inferencing, no SPARQL. RDF/JSON-LD
// export is a separate, optional edge (semstreams vocabulary/export), built only
// if a consumer needs it. See docs/adr/0005-bfo-cco-ontology-alignment.md.
package ontology

import (
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/source/ast"
)

// ClassPredicate carries an entity's BFO/CCO class IRI as an rdf:type assertion.
// StampClass auto-populates it; a value already present on the entity is treated
// as an explicit operator override and left untouched (first-writer-wins),
// mirroring semstreams ADR-042's operator-override predicate.
const ClassPredicate = "entity.ontology.class"

// classKey identifies an entity kind by the (domain, type) segments of its
// 6-part ID. Both are required because type strings collide across domains:
// "package" is a Go package (code) vs an npm manifest (config); "image" is a
// media file (media) vs a container reference (config).
type classKey struct{ domain, typ string }

// sourceClasses maps non-code entity kinds (git/web/config/media) to BFO/CCO
// class IRIs. Type strings match the handlers' entityid.Build calls.
var sourceClasses = map[classKey]string{
	{"git", "commit"}: cco.Act,
	{"git", "author"}: cco.Person,
	{"git", "branch"}: cco.DesignativeInformationContentEntity,

	{"web", "page"}: cco.Document,
	{"web", "doc"}:  cco.Document,

	{"config", "gomod"}:      cco.Specification,
	{"config", "project"}:    cco.Specification,
	{"config", "package"}:    cco.Specification,
	{"config", "dependency"}: cco.DesignativeInformationContentEntity,
	{"config", "image"}:      cco.Identifier,

	{"media", "image"}:    cco.InformationBearingArtifact,
	{"media", "video"}:    cco.InformationBearingArtifact,
	{"media", "audio"}:    cco.InformationBearingArtifact,
	{"media", "keyframe"}: cco.InformationBearingArtifact,
	{"media", "blob"}:     cco.InformationBearingArtifact,
}

// codeTypeClasses maps AST code entity types to BFO/CCO class IRIs. Keyed by the
// ast.CodeEntityType value (domain is the language, e.g. golang/typescript).
// Functions/methods are procedures (Algorithm); other symbols are SoftwareCode;
// files/folders bear information; a repo is an artifact. Judgment calls per
// ADR-0005 — documented and override-able.
var codeTypeClasses = map[ast.CodeEntityType]string{
	ast.TypeFunction:  cco.Algorithm,
	ast.TypeMethod:    cco.Algorithm,
	ast.TypeStruct:    cco.SoftwareCode,
	ast.TypeInterface: cco.SoftwareCode,
	ast.TypeClass:     cco.SoftwareCode,
	ast.TypeEnum:      cco.SoftwareCode,
	ast.TypeType:      cco.SoftwareCode,
	ast.TypeConst:     cco.SoftwareCode,
	ast.TypeVar:       cco.SoftwareCode,
	ast.TypeComponent: cco.SoftwareCode,
	ast.TypePackage:   cco.SoftwareCode,
	ast.TypeFile:      cco.InformationBearingArtifact,
	ast.TypeFolder:    cco.InformationBearingArtifact,
	ast.TypeRepo:      cco.Artifact,
}

// ClassFor returns the BFO/CCO class IRI for an entity kind identified by the
// domain and type segments of its 6-part ID, and whether a mapping exists.
// Non-code domains are matched first so a code "package" (under a language
// domain) and a config "package" resolve distinctly.
func ClassFor(domain, entityType string) (string, bool) {
	if iri, ok := sourceClasses[classKey{domain, entityType}]; ok {
		return iri, true
	}
	if iri, ok := codeTypeClasses[ast.CodeEntityType(entityType)]; ok {
		return iri, true
	}
	return "", false
}
