// Package source provides vocabulary predicates for document, repository, and web sources.
//
// This vocabulary supports the sources and knowledge ingestion system defined in
// ADR-006, enabling:
//   - Document metadata extraction (SOPs, specs, datasheets, references)
//   - Repository source tracking for external code ingestion
//   - Web source ingestion from documentation sites
//   - Parent-chunk relationships for large document storage
//   - Context assembly for reviewer prompts
//
// # Semstreams Integration
//
// This package follows semstreams vocabulary patterns:
//   - Predicates use three-level dotted notation (domain.category.property)
//   - Predicates are registered in init() using vocabulary.Register()
//   - IRI mappings use vocabulary.WithIRI() for RDF export compatibility
//   - Metadata includes description, data type, and range where applicable
//
// # Document Categories
//
// Documents are classified by category:
//   - sop: Standard Operating Procedure (validation rules)
//   - spec: Technical specification
//   - datasheet: Data format documentation
//   - reference: General reference material
//   - api: API documentation
//
// # Document Severity (SOPs)
//
// SOPs include severity levels for review enforcement:
//   - error: Must be addressed; blocks approval
//   - warning: Should be addressed; reviewer discretion
//   - info: Informational; no enforcement
//
// # Parent-Chunk Model
//
// Large documents are split into chunks for context budget management:
//
//	Parent Entity: source.doc.{category}.{slug}
//	  - Metadata: category, applies_to, severity, summary, requirements
//	  - No full content (only summary/requirements for budget efficiency)
//
//	Chunk Entities: source.doc.{category}.{slug}.chunk.{n}
//	  - Full content for that section
//	  - Linked via code.structure.belongs to parent
//	  - Section name and chunk index tracked
//
// # Repository Sources
//
// External repositories can be added as sources for code ingestion:
//   - Git URL and branch tracking
//   - Auto-pull configuration for staleness detection
//   - Language detection and entity counts
//
// # Web Sources
//
// Web pages can be ingested for documentation and reference:
//   - URL and content type tracking
//   - ETag-based staleness detection for efficient refresh
//   - Title extraction from HTML
//   - Auto-refresh with configurable intervals
//   - Content chunking for context assembly
//
// Web Source Entity Model:
//
//	Parent Entity: source.web.{domain}-{path-slug}
//	  - URL, title, content type, ETag, timestamps
//	  - Auto-refresh configuration
//
//	Chunk Entities: source.web.{domain}-{path-slug}.chunk.{n}
//	  - Markdown content sections
//	  - Section headings preserved
//	  - Linked via code.structure.belongs to parent
//
// # Usage
//
// Import the package to register predicates, then use predicate constants:
//
//	import (
//	    "github.com/c360studio/semsource/source/vocabulary"
//	    "github.com/c360studio/semstreams/message"
//	)
//
//	func (p *Processor) buildDocumentTriples(doc Document) []message.Triple {
//	    return []message.Triple{
//	        {Subject: doc.ID, Predicate: source.DocCategory, Object: "sop"},
//	        {Subject: doc.ID, Predicate: source.DocAppliesTo, Object: "*.go"},
//	        {Subject: doc.ID, Predicate: source.DocSeverity, Object: "error"},
//	        {Subject: doc.ID, Predicate: source.DocSummary, Object: doc.Summary},
//	        {Subject: doc.ID, Predicate: source.DocRequirements, Object: doc.Requirements},
//	    }
//	}
//
// # IRI Mappings
//
// The package registers IRI mappings to standard ontologies:
//   - DocSummary → dc:abstract
//   - DocMimeType → dc:format
//   - DocCategory → dc:type
//   - Parent-chunk relationships use BFO part_of/has_part via code.structure.belongs
//
// Semspec-specific predicates use: https://semspec.dev/ontology/source/
package source
