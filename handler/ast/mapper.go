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
// All scalar fields from CodeEntity are mapped to Properties. Relationship
// fields (calls, implements, imports, embeds, etc.) are expressed as RawEdge
// values so the normalizer can resolve full entity IDs from instance hints.
func mapCodeEntity(ce *semsourceast.CodeEntity, domain, system string) handler.RawEntity {
	props := map[string]any{
		"path":       ce.Path,
		"package":    ce.Package,
		"language":   ce.Language,
		"visibility": string(ce.Visibility),
	}

	if ce.StartLine > 0 {
		props["start_line"] = ce.StartLine
	}
	if ce.EndLine > 0 {
		props["end_line"] = ce.EndLine
	}
	if ce.StartLine > 0 && ce.EndLine > 0 {
		props["line_count"] = ce.EndLine - ce.StartLine + 1
	}
	if ce.DocComment != "" {
		props["doc_comment"] = ce.DocComment
	}
	if ce.Hash != "" {
		props["hash"] = ce.Hash
	}
	if ce.Framework != "" {
		props["framework"] = ce.Framework
	}
	if ce.Receiver != "" {
		props["receiver"] = ce.Receiver
	}

	// Capability metadata is flattened into properties with a "capability." prefix.
	if cap := ce.Capability; cap != nil {
		if cap.Name != "" {
			props["capability.name"] = cap.Name
		}
		if cap.Description != "" {
			props["capability.description"] = cap.Description
		}
		if len(cap.Tools) > 0 {
			props["capability.tools"] = cap.Tools
		}
		if len(cap.Inputs) > 0 {
			props["capability.inputs"] = cap.Inputs
		}
		if len(cap.Outputs) > 0 {
			props["capability.outputs"] = cap.Outputs
		}
	}

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     domain,
		System:     system,
		EntityType: string(ce.Type),
		Instance:   ce.Name,
		Properties: props,
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
	case "java":
		return "java"
	case "python":
		return "python"
	case "svelte":
		return "svelte"
	default: // "go"
		return handler.DomainGolang
	}
}
