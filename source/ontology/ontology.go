// Package ontology aligns semsource entity kinds and predicates to the BFO 2020
// and CCO v2.2 upper ontologies for standards-aligned ("standards at work")
// stamping. The IRI constants and superclass hierarchy live in semvocab
// (catalog/bfo, catalog/cco); this package owns only the semsource-specific
// class mapping and the triple that stamps it.
//
// Ontology-specificity ranking lives in the pkg/fusion engine via
// fusionvocab.New(cco.Parents). Every class stamped here is a CCO v2.2 constant
// whose cco.Parents(iri) chain terminates at BFO 2020 Entity (BFO_0000001).
//
// It is alignment, not reasoning: no OWL inferencing, no SPARQL. See
// docs/adr/0005-bfo-cco-ontology-alignment.md.
package ontology

import (
	"github.com/c360studio/semvocab/catalog/cco"

	"github.com/c360studio/semsource/source/ast"
)

// ClassPredicate carries an entity's BFO/CCO class IRI as an rdf:type assertion.
// StampClass auto-populates it; a value already present on the entity is treated
// as an explicit operator override and left untouched (first-writer-wins),
// mirroring ADR-042's operator-override predicate.
const ClassPredicate = "entity.ontology.class"

// classKey identifies an entity kind by the (domain, type) segments of its
// 6-part ID. Both are required because type strings collide across domains:
// "package" is a Go package (code) vs an npm manifest (config); "image" is a
// media file (media) vs a container reference (config).
type classKey struct{ domain, typ string }

// sourceClasses maps non-code entity kinds (git/web/config/media) to CCO v2.2
// class IRIs. Type strings match the handlers' entityid.Build calls.
var sourceClasses = map[classKey]string{
	{"git", "commit"}: cco.Act,
	{"git", "author"}: cco.Person,
	{"git", "branch"}: cco.DesignativeInformationContentEntity,

	{"web", "page"}: cco.MaterialCopyOfDocument,
	{"web", "doc"}:  cco.MaterialCopyOfDocument,

	{"config", "gomod"}:      cco.PrescriptiveInformationContentEntity,
	{"config", "project"}:    cco.PrescriptiveInformationContentEntity,
	{"config", "package"}:    cco.PrescriptiveInformationContentEntity,
	{"config", "dependency"}: cco.DesignativeInformationContentEntity,
	{"config", "image"}:      cco.DesignativeInformationContentEntity,

	{"media", "image"}:    cco.InformationBearingArtifact,
	{"media", "video"}:    cco.InformationBearingArtifact,
	{"media", "audio"}:    cco.InformationBearingArtifact,
	{"media", "keyframe"}: cco.InformationBearingArtifact,
	{"media", "blob"}:     cco.InformationBearingArtifact,
}

// codeTypeClasses maps AST code entity types to CCO v2.2 class IRIs. Keyed by
// the ast.CodeEntityType value (domain is the language, e.g. golang/typescript).
// Functions/methods are procedures (Algorithm, depth 4); non-function code
// declarations are PrescriptiveInformationContentEntity (direct parent of
// Algorithm at depth 3, preserving function/method specificity); files/folders
// bear information; a repo is a MaterialArtifact. Judgment calls per ADR-0005 —
// documented and override-able.
var codeTypeClasses = map[ast.CodeEntityType]string{
	ast.TypeFunction:  cco.Algorithm,
	ast.TypeMethod:    cco.Algorithm,
	ast.TypeStruct:    cco.PrescriptiveInformationContentEntity,
	ast.TypeInterface: cco.PrescriptiveInformationContentEntity,
	ast.TypeClass:     cco.PrescriptiveInformationContentEntity,
	ast.TypeEnum:      cco.PrescriptiveInformationContentEntity,
	ast.TypeType:      cco.PrescriptiveInformationContentEntity,
	ast.TypeConst:     cco.PrescriptiveInformationContentEntity,
	ast.TypeVar:       cco.PrescriptiveInformationContentEntity,
	ast.TypeComponent: cco.PrescriptiveInformationContentEntity,
	ast.TypePackage:   cco.PrescriptiveInformationContentEntity,
	ast.TypeFile:      cco.InformationBearingArtifact,
	ast.TypeFolder:    cco.InformationBearingArtifact,
	ast.TypeRepo:      cco.MaterialArtifact,
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
