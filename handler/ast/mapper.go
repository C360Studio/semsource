package ast

import (
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/handler"
)

// mapParseResult converts a ParseResult into a slice of RawEntity values.
// The system slug is the NATS-safe identifier for the source path.
func mapParseResult(result *semsourceast.ParseResult, lang, system string) []handler.RawEntity {
	if result == nil {
		return nil
	}

	domain := langToDomain(lang)
	entities := make([]handler.RawEntity, 0, len(result.Entities))

	for _, ce := range result.Entities {
		if ce == nil {
			continue
		}
		raw := mapCodeEntity(ce, domain, system)
		entities = append(entities, raw)
	}
	return entities
}

// mapCodeEntity converts a single CodeEntity to a RawEntity.
func mapCodeEntity(ce *semsourceast.CodeEntity, domain, system string) handler.RawEntity {
	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     domain,
		System:     system,
		EntityType: string(ce.Type),
		Instance:   ce.Name,
		Properties: map[string]any{
			"path":       ce.Path,
			"package":    ce.Package,
			"language":   ce.Language,
			"visibility": string(ce.Visibility),
		},
		// Carry the pre-formed triples from the entity directly.
		// The normalizer will merge these with any properties-derived triples.
		Triples: ce.Triples(),
	}

	if ce.StartLine > 0 {
		raw.Properties["start_line"] = ce.StartLine
	}
	if ce.EndLine > 0 {
		raw.Properties["end_line"] = ce.EndLine
	}
	if ce.DocComment != "" {
		raw.Properties["doc_comment"] = ce.DocComment
	}

	// Map structural edges from the CodeEntity to RawEdge values.
	raw.Edges = codeEntityEdges(ce)

	return raw
}

// codeEntityEdges extracts RawEdge values from the relationship fields of a CodeEntity.
func codeEntityEdges(ce *semsourceast.CodeEntity) []handler.RawEdge {
	var edges []handler.RawEdge

	for _, callee := range ce.Calls {
		edges = append(edges, handler.RawEdge{
			FromHint: ce.Name,
			ToHint:   callee,
			EdgeType: "calls",
			Weight:   1.0,
		})
	}
	for _, iface := range ce.Implements {
		edges = append(edges, handler.RawEdge{
			FromHint: ce.Name,
			ToHint:   iface,
			EdgeType: "implements",
			Weight:   1.0,
		})
	}
	for _, imp := range ce.Imports {
		edges = append(edges, handler.RawEdge{
			FromHint: ce.Name,
			ToHint:   imp,
			EdgeType: "imports",
			Weight:   0.5,
		})
	}
	for _, embedded := range ce.Embeds {
		edges = append(edges, handler.RawEdge{
			FromHint: ce.Name,
			ToHint:   embedded,
			EdgeType: "embeds",
			Weight:   1.0,
		})
	}

	return edges
}

// langToDomain maps a language name to the handler domain constant.
func langToDomain(lang string) string {
	switch lang {
	case "ts", "typescript", "javascript":
		return "typescript"
	default: // "go"
		return handler.DomainGolang
	}
}
