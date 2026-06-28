package ontology

import (
	"github.com/c360studio/semstreams/vocabulary/bfo"
	"github.com/c360studio/semstreams/vocabulary/cco"
)

// parent maps a BFO/CCO class IRI to its immediate superclass — a minimal,
// hand-encoded slice of the (fixed, standard) BFO/CCO subclass tree covering
// only the classes ClassFor emits. It is a static table, NOT a reasoner: it
// exists so the ranker can measure ontology distance and specificity.
//
// Upstreaming a real SubClassOf map into semstreams vocabulary/bfo|cco is
// tracked in docs/upstream/semstreams-asks.md (framework-shaped); this is the
// local stopgap.
var parent = map[string]string{
	// Continuant spine.
	bfo.Continuant:                     bfo.Entity,
	bfo.Occurrent:                      bfo.Entity,
	bfo.GenericallyDependentContinuant: bfo.Continuant,
	bfo.IndependentContinuant:          bfo.Continuant,
	bfo.MaterialEntity:                 bfo.IndependentContinuant,
	bfo.Object:                         bfo.MaterialEntity,
	bfo.Process:                        bfo.Occurrent,

	// Information Content Entity branch.
	cco.InformationContentEntity:            bfo.GenericallyDependentContinuant,
	cco.DirectiveInformationContentEntity:   cco.InformationContentEntity,
	cco.DescriptiveInformationContentEntity: cco.InformationContentEntity,
	cco.DesignativeInformationContentEntity: cco.InformationContentEntity,
	cco.Specification:                       cco.DirectiveInformationContentEntity,
	cco.Requirement:                         cco.DirectiveInformationContentEntity,
	cco.PlanSpecification:                   cco.DirectiveInformationContentEntity,
	cco.Rule:                                cco.DirectiveInformationContentEntity,
	cco.Objective:                           cco.DirectiveInformationContentEntity,
	cco.SoftwareCode:                        cco.DirectiveInformationContentEntity,
	cco.Algorithm:                           cco.DirectiveInformationContentEntity,
	cco.Identifier:                          cco.DesignativeInformationContentEntity,
	cco.Name:                                cco.DesignativeInformationContentEntity,
	cco.MeasurementInformationContentEntity: cco.DescriptiveInformationContentEntity,

	// Artifact branch.
	cco.Artifact:                   bfo.Object,
	cco.InformationBearingArtifact: cco.Artifact,
	cco.Document:                   cco.InformationBearingArtifact,

	// Agent branch.
	cco.Agent:        bfo.Object,
	cco.Person:       cco.Agent,
	cco.Organization: cco.Agent,

	// Act branch.
	cco.Act: bfo.Process,
}

// Ancestors returns iri followed by each superclass up to the root (bfo.Entity).
// An unknown IRI returns just itself. The visited guard ensures a malformed
// `parent` edit (a cycle) terminates instead of hanging the rank-time loop.
func Ancestors(iri string) []string {
	chain := []string{iri}
	seen := map[string]struct{}{iri: {}}
	for {
		p, ok := parent[iri]
		if !ok {
			return chain
		}
		if _, dup := seen[p]; dup {
			return chain // cycle guard: stop rather than loop forever
		}
		seen[p] = struct{}{}
		chain = append(chain, p)
		iri = p
	}
}

// Depth returns the number of hops from iri up to the root (bfo.Entity is 0).
// More specific classes are deeper. An unknown IRI returns 0.
func Depth(iri string) int {
	return len(Ancestors(iri)) - 1
}

// Distance returns the number of subclass hops between two classes via their
// lowest common ancestor. Identical classes are 0; classes with no shared
// ancestor (e.g. an unknown IRI) return -1.
func Distance(a, b string) int {
	if a == b {
		return 0
	}
	depthOf := map[string]int{}
	for d, anc := range Ancestors(a) {
		depthOf[anc] = d
	}
	for d, anc := range Ancestors(b) {
		if da, ok := depthOf[anc]; ok {
			return da + d
		}
	}
	return -1
}
